package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/auth"
	mailrules "github.com/zenexcloud/zenex-panel/apps/api/internal/mail"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// MailStore keeps mail settings and mailboxes. *store.Store satisfies it.
type MailStore interface {
	ZoneOwner(ctx context.Context, zoneID string) (store.ZoneInfo, error)
	GetMailDomain(ctx context.Context, zoneID string) (store.MailDomain, error)
	SetMailDomain(ctx context.Context, zoneID string, enabled bool) error
	SetMailDKIM(ctx context.Context, zoneID, value string) error
	ListMailboxes(ctx context.Context, zoneID string) ([]store.Mailbox, error)
	CreateMailbox(ctx context.Context, mb store.Mailbox) (store.Mailbox, error)
	GetMailbox(ctx context.Context, id string) (store.Mailbox, error)
	UpdateMailbox(ctx context.Context, mb store.Mailbox) error
	DeleteMailbox(ctx context.Context, id string) error
}

// MailResolver answers the DNS lookups of the mail check. Nil means the system resolver.
type MailResolver = mailrules.Resolver

// MailServerOps sets up the mail server and creates DKIM keys. *manage.Manager satisfies it.
type MailServerOps interface {
	ApplyMailServer(ctx context.Context) error
	MailDKIMValue(ctx context.Context, apex string) (string, error)
}

var (
	ErrMailDisabled = APIError{Status: http.StatusConflict, Code: "mail_disabled", Message: "turn on mail for this domain before adding mailboxes"}
	ErrAddressTaken = APIError{Status: http.StatusConflict, Code: "address_taken", Message: "that mailbox already exists on this domain"}
)

// mailView is what the Mail section of a domain shows.
type mailView struct {
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
	// DeliveryReady is false until the mail server is installed (phase 2). Mailboxes are
	// saved now and start receiving mail then.
	DeliveryReady bool               `json:"delivery_ready"`
	Mailboxes     []store.Mailbox    `json:"mailboxes"`
	DNS           []mailrules.Record `json:"dns"`
}

// loadZoneForUser returns a connected domain the caller may manage (owner or administrator).
// Anyone else gets 404, so domain IDs cannot be probed.
func (d Deps) loadZoneForUser(w http.ResponseWriter, r *http.Request) (store.ZoneInfo, *store.User, bool) {
	if d.Mail == nil {
		writeError(w, requestIDFrom(r), ErrUnavailable)
		return store.ZoneInfo{}, nil, false
	}
	user, _ := currentUser(r)
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return store.ZoneInfo{}, nil, false
	}
	zone, err := d.Mail.ZoneOwner(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !canAccess(user, zone.OwnerID)) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return store.ZoneInfo{}, nil, false
	}
	if err != nil {
		d.internal(w, r, "mail.zone", err)
		return store.ZoneInfo{}, nil, false
	}
	return zone, user, true
}

// buildMailView reads the mail state of a connected domain.
func (d Deps) buildMailView(ctx context.Context, zone store.ZoneInfo) (mailView, error) {
	md, err := d.Mail.GetMailDomain(ctx, zone.ID)
	if err != nil {
		return mailView{}, err
	}
	boxes, err := d.Mail.ListMailboxes(ctx, zone.ID)
	if err != nil {
		return mailView{}, err
	}
	return mailView{
		Domain:    zone.Apex,
		Enabled:   md.Enabled,
		Mailboxes: boxes,
		DNS:       mailrules.DNSRecords(zone.Apex, d.Site.PublicIP, md.DKIMPublic),
	}, nil
}

func (d Deps) handleGetMail(w http.ResponseWriter, r *http.Request) {
	zone, _, ok := d.loadZoneForUser(w, r)
	if !ok {
		return
	}
	view, err := d.buildMailView(r.Context(), zone)
	if err != nil {
		d.internal(w, r, "mail.get", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type mailSwitchRequest struct {
	Enabled bool `json:"enabled"`
}

func (d Deps) handleSetMail(w http.ResponseWriter, r *http.Request) {
	zone, user, ok := d.loadZoneForUser(w, r)
	if !ok {
		return
	}
	var in mailSwitchRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	// Turning mail on creates the domain's DKIM key (once) and stores its public part, so the
	// DNS records can show it. Without a mail server on this host there is no key yet.
	if in.Enabled && d.MailServer != nil {
		value, err := d.MailServer.MailDKIMValue(r.Context(), zone.Apex)
		if err != nil {
			writeError(w, requestIDFrom(r), APIError{Status: http.StatusUnprocessableEntity, Code: "dkim_failed", Message: "the signing key could not be created: " + err.Error()})
			return
		}
		if err := d.Mail.SetMailDKIM(r.Context(), zone.ID, value); err != nil {
			d.internal(w, r, "mail.dkim", err)
			return
		}
	}
	if err := d.Mail.SetMailDomain(r.Context(), zone.ID, in.Enabled); err != nil {
		d.internal(w, r, "mail.set", err)
		return
	}
	action := "mail.disable"
	if in.Enabled {
		action = "mail.enable"
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: action, TargetType: "domain", TargetID: zone.ID, Result: "success"})
	view, err := d.buildMailView(r.Context(), zone)
	if err != nil {
		d.internal(w, r, "mail.get", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

type mailboxCreateRequest struct {
	LocalPart string `json:"local_part"`
	Password  string `json:"password"`
	QuotaMB   int    `json:"quota_mb"`
}

func (d Deps) handleCreateMailbox(w http.ResponseWriter, r *http.Request) {
	zone, user, ok := d.loadZoneForUser(w, r)
	if !ok {
		return
	}
	var in mailboxCreateRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if in.QuotaMB == 0 {
		in.QuotaMB = mailrules.DefaultQuotaMB
	}
	if !mailrules.ValidLocalPart(in.LocalPart) {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadRequest, Code: "invalid_mailbox", Message: "use lower-case letters, digits, dots, dashes or underscores before the @"})
		return
	}
	if err := mailrules.ValidPassword(in.Password); err != nil {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadRequest, Code: "invalid_password", Message: err.Error()})
		return
	}
	if !mailrules.ValidQuota(in.QuotaMB) {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadRequest, Code: "invalid_quota", Message: "the quota must be between 10 MB and 100 GB"})
		return
	}
	md, err := d.Mail.GetMailDomain(r.Context(), zone.ID)
	if err != nil {
		d.internal(w, r, "mail.get", err)
		return
	}
	if !md.Enabled {
		writeError(w, requestIDFrom(r), ErrMailDisabled)
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		d.internal(w, r, "mail.hash", err)
		return
	}
	mb, err := d.Mail.CreateMailbox(r.Context(), store.Mailbox{
		ZoneID: zone.ID, LocalPart: in.LocalPart, PasswordHash: hash, QuotaMB: in.QuotaMB, Active: true,
	})
	if errors.Is(err, store.ErrConflict) {
		writeError(w, requestIDFrom(r), ErrAddressTaken)
		return
	}
	if err != nil {
		d.internal(w, r, "mail.create", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "mailbox.create", TargetType: "mailbox", TargetID: mb.ID, Result: "success"})
	writeJSON(w, http.StatusCreated, mb)
}

// loadMailboxForUser returns a mailbox whose domain the caller may manage.
func (d Deps) loadMailboxForUser(w http.ResponseWriter, r *http.Request) (store.Mailbox, *store.User, bool) {
	if d.Mail == nil {
		writeError(w, requestIDFrom(r), ErrUnavailable)
		return store.Mailbox{}, nil, false
	}
	user, _ := currentUser(r)
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return store.Mailbox{}, nil, false
	}
	mb, err := d.Mail.GetMailbox(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return store.Mailbox{}, nil, false
	}
	if err != nil {
		d.internal(w, r, "mail.mailbox", err)
		return store.Mailbox{}, nil, false
	}
	zone, err := d.Mail.ZoneOwner(r.Context(), mb.ZoneID)
	if err != nil || !canAccess(user, zone.OwnerID) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return store.Mailbox{}, nil, false
	}
	return mb, user, true
}

// mailboxUpdateRequest holds only the fields that may change. Nil means "leave as it is".
type mailboxUpdateRequest struct {
	Password *string `json:"password"`
	QuotaMB  *int    `json:"quota_mb"`
	Active   *bool   `json:"active"`
}

func (d Deps) handleUpdateMailbox(w http.ResponseWriter, r *http.Request) {
	mb, user, ok := d.loadMailboxForUser(w, r)
	if !ok {
		return
	}
	var in mailboxUpdateRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if in.Password != nil {
		if err := mailrules.ValidPassword(*in.Password); err != nil {
			writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadRequest, Code: "invalid_password", Message: err.Error()})
			return
		}
		hash, err := auth.HashPassword(*in.Password)
		if err != nil {
			d.internal(w, r, "mail.hash", err)
			return
		}
		mb.PasswordHash = hash
	}
	if in.QuotaMB != nil {
		if !mailrules.ValidQuota(*in.QuotaMB) {
			writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadRequest, Code: "invalid_quota", Message: "the quota must be between 10 MB and 100 GB"})
			return
		}
		mb.QuotaMB = *in.QuotaMB
	}
	if in.Active != nil {
		mb.Active = *in.Active
	}
	if err := d.Mail.UpdateMailbox(r.Context(), mb); err != nil {
		d.internal(w, r, "mail.update", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "mailbox.update", TargetType: "mailbox", TargetID: mb.ID, Result: "success"})
	writeJSON(w, http.StatusOK, mb)
}

func (d Deps) handleDeleteMailbox(w http.ResponseWriter, r *http.Request) {
	mb, user, ok := d.loadMailboxForUser(w, r)
	if !ok {
		return
	}
	if err := d.Mail.DeleteMailbox(r.Context(), mb.ID); err != nil {
		d.internal(w, r, "mail.delete", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "mailbox.delete", TargetType: "mailbox", TargetID: mb.ID, Result: "success"})
	w.WriteHeader(http.StatusNoContent)
}

// handleApplyMailServer writes the mail server configuration and reloads the services.
// Administrators only: it changes the server's mail setup for every hosted domain.
func (d Deps) handleApplyMailServer(w http.ResponseWriter, r *http.Request) {
	user, ok := d.requireAdminUser(w, r)
	if !ok {
		return
	}
	if d.MailServer == nil {
		writeError(w, requestIDFrom(r), ErrUnavailable)
		return
	}
	if err := d.MailServer.ApplyMailServer(r.Context()); err != nil {
		d.manageError(w, r, "mail.server.apply", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "mail.server.apply", TargetType: "settings", TargetID: "mail_server", Result: "success"})
	writeJSON(w, http.StatusOK, map[string]bool{"applied": true})
}

// handleCheckMailDNS looks up the records of a domain's mail setup on public DNS. It reports
// what is published; it changes nothing.
func (d Deps) handleCheckMailDNS(w http.ResponseWriter, r *http.Request) {
	zone, _, ok := d.loadZoneForUser(w, r)
	if !ok {
		return
	}
	md, err := d.Mail.GetMailDomain(r.Context(), zone.ID)
	if err != nil {
		d.internal(w, r, "mail.check", err)
		return
	}
	var resolver MailResolver = net.DefaultResolver
	if d.MailResolver != nil {
		resolver = d.MailResolver
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	results := mailrules.CheckRecords(ctx, resolver, zone.Apex, d.Site.PublicIP, md.DKIMPublic)
	allOK := true
	for _, res := range results {
		allOK = allOK && res.Status == mailrules.StatusOK
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"checked_at": time.Now().UTC(),
		"all_ok":     allOK,
		"results":    results,
	})
}
