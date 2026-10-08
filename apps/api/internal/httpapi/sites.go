package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/dnscheck"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/provision"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/packages/validation"
)

var (
	ErrDomainNotConnected = APIError{Status: http.StatusBadRequest, Code: "domain_not_connected", Message: "connect this domain first, then create websites under it"}
	ErrSlugTooLong        = APIError{Status: http.StatusBadRequest, Code: "label_too_long", Message: "the website name can be at most 28 characters"}
	ErrInvalidLabel       = APIError{Status: http.StatusBadRequest, Code: "invalid_label", Message: "use 3-32 lowercase letters, digits and hyphens; start with a letter"}
	ErrInvalidDomainName  = APIError{Status: http.StatusBadRequest, Code: "invalid_domain", Message: "enter a valid domain such as example.com"}
	ErrIdempotencyNeeded  = APIError{Status: http.StatusBadRequest, Code: "idempotency_key_required", Message: "missing Idempotency-Key header"}
	ErrAlreadyExists      = APIError{Status: http.StatusConflict, Code: "already_exists", Message: "this address is already in use"}
	ErrDomainTaken        = APIError{Status: http.StatusConflict, Code: "domain_taken", Message: "this domain is already connected to an account"}
	ErrNotRetryable       = APIError{Status: http.StatusConflict, Code: "not_retryable", Message: "only a failed job can be retried"}
	ErrNodeMissing        = APIError{Status: http.StatusServiceUnavailable, Code: "node_missing", Message: "this server is not registered yet; sign in with the administrator account"}
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// SiteStore is the persistence surface for domains, sites and jobs.
type SiteStore interface {
	NotificationStore
	GetBranding(ctx context.Context) (store.Branding, error)
	SetBranding(ctx context.Context, userID string, b store.Branding) error
	CreateDomain(ctx context.Context, ownerID, apex string) (store.Domain, error)
	ListDomains(ctx context.Context, ownerID string) ([]store.Domain, error)
	DomainOwnedBy(ctx context.Context, ownerID, apex string) (string, error)
	CreateSiteWithJob(ctx context.Context, in store.NewSite) (string, string, bool, error)
	GetSite(ctx context.Context, id string) (store.Site, error)
	ListSites(ctx context.Context, ownerID string) ([]store.Site, error)
	LatestJobForSite(ctx context.Context, siteID string) (store.Job, error)
	GetJob(ctx context.Context, id string) (store.Job, error)
	JobSteps(ctx context.Context, jobID string) ([]store.JobStep, error)
	RequeueFailedJob(ctx context.Context, jobID string) (bool, error)
	GetDomainOwned(ctx context.Context, ownerID, id string) (store.Domain, error)
	SetDomainCheck(ctx context.Context, id string, verified bool, message string) error
	FindSiteByIdempotencyKey(ctx context.Context, actorID, key string) (string, string, error)
}

// SiteSettings are the server-level values the site handlers need.
type SiteSettings struct {
	NodeID     string
	PHPVersion string
	SecretKey  []byte
	// DNS checks that customer domains point at this server.
	DNS *dnscheck.Checker
	// StartJob runs a job in the background.
	StartJob func(jobID string)
}

func isAdmin(u *store.User) bool {
	for _, r := range u.Roles {
		if r == "administrator" {
			return true
		}
	}
	return false
}

// canAccess reports whether the user may see a resource owned by ownerID.
func canAccess(u *store.User, ownerID string) bool {
	return isAdmin(u) || u.ID == ownerID
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// ---------------------------------------------------------------------------
// Domains
// ---------------------------------------------------------------------------

type domainRequest struct {
	Apex string `json:"apex"`
}

func (d Deps) handleListDomains(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	list, err := d.Sites.ListDomains(r.Context(), user.ID)
	if err != nil {
		d.internal(w, r, "domains.list", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (d Deps) handleConnectDomain(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	var in domainRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	apex := strings.ToLower(strings.TrimSpace(in.Apex))
	if err := validation.DomainName(apex); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidDomainName)
		return
	}
	dom, err := d.Sites.CreateDomain(r.Context(), user.ID, apex)
	if errors.Is(err, store.ErrConflict) {
		d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, Action: "domain.connect", TargetType: "domain", TargetID: apex, Result: "denied", ErrorCode: ErrDomainTaken.Code})
		writeError(w, requestIDFrom(r), ErrDomainTaken)
		return
	}
	if err != nil {
		d.internal(w, r, "domains.connect", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "domain.connect", TargetType: "domain", TargetID: dom.ID, Result: "success"})
	writeJSON(w, http.StatusCreated, d.checkDomain(r, user.ID, dom))
}

// checkDomain runs the DNS check, stores the result and returns the updated domain.
func (d Deps) checkDomain(r *http.Request, ownerID string, dom store.Domain) store.Domain {
	if d.Site.DNS == nil {
		return dom
	}
	res := d.Site.DNS.CheckDomain(r.Context(), dom.Apex)
	if dom.Verified != res.Verified {
		if res.Verified {
			d.notify(r, ownerID, "success", "DNS is ready for "+dom.Apex, res.Message)
		} else {
			d.notify(r, ownerID, "warning", "DNS problem for "+dom.Apex, res.Message)
		}
	}
	if err := d.Sites.SetDomainCheck(r.Context(), dom.ID, res.Verified, res.Message); err != nil {
		d.Log.Error("saving domain check failed", "request_id", requestIDFrom(r), "operation", "domains.check", "error", err)
		dom.Verified, dom.Message = res.Verified, res.Message
		return dom
	}
	if updated, err := d.Sites.GetDomainOwned(r.Context(), ownerID, dom.ID); err == nil {
		return updated
	}
	dom.Verified, dom.Message = res.Verified, res.Message
	return dom
}

// handleVerifyDomain runs the DNS check again, after the customer changed DNS.
func (d Deps) handleVerifyDomain(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return
	}
	dom, err := d.Sites.GetDomainOwned(r.Context(), user.ID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return
	}
	if err != nil {
		d.internal(w, r, "domains.verify", err)
		return
	}
	dom = d.checkDomain(r, user.ID, dom)
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "domain.verify", TargetType: "domain", TargetID: dom.ID, Result: "success"})
	writeJSON(w, http.StatusOK, dom)
}

// ---------------------------------------------------------------------------
// Sites
// ---------------------------------------------------------------------------

type createSiteRequest struct {
	Label string `json:"label"`
	Apex  string `json:"apex"`
}

type siteResponse struct {
	Site  store.Site `json:"site"`
	JobID string     `json:"job_id"`
}

func (d Deps) handleListSites(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	owner := user.ID
	if isAdmin(user) {
		owner = ""
	}
	list, err := d.Sites.ListSites(r.Context(), owner)
	if err != nil {
		d.internal(w, r, "sites.list", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (d Deps) handleCreateSite(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	reqID := requestIDFrom(r)

	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(w, reqID, ErrIdempotencyNeeded)
		return
	}
	if err := validation.IdempotencyKey(key); err != nil {
		writeError(w, reqID, ErrIdempotencyNeeded)
		return
	}
	// A repeated request with the same key returns the original site, without
	// running DNS checks again or creating anything.
	if siteID, jobID, err := d.Sites.FindSiteByIdempotencyKey(r.Context(), user.ID, key); err == nil {
		if site, gerr := d.Sites.GetSite(r.Context(), siteID); gerr == nil {
			writeJSON(w, http.StatusOK, siteResponse{Site: site, JobID: jobID})
			return
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		d.internal(w, r, "sites.create", err)
		return
	}

	if d.Site.NodeID == "" {
		writeError(w, reqID, ErrNodeMissing)
		return
	}

	var in createSiteRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, reqID, ErrInvalidRequest)
		return
	}
	label := strings.ToLower(strings.TrimSpace(in.Label))
	apex := strings.ToLower(strings.TrimSpace(in.Apex))
	if len(label) > 28 {
		writeError(w, reqID, ErrSlugTooLong)
		return
	}
	domain, err := validation.ComposeSiteDomain(label, apex)
	if errors.Is(err, validation.ErrInvalidDomain) {
		writeError(w, reqID, ErrInvalidDomainName)
		return
	}
	if err != nil {
		writeError(w, reqID, ErrInvalidLabel)
		return
	}
	zoneID, err := d.Sites.DomainOwnedBy(r.Context(), user.ID, apex)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, reqID, ErrDomainNotConnected)
		return
	}
	if err != nil {
		d.internal(w, r, "sites.create", err)
		return
	}

	// The website name itself must resolve to this server before anything is built.
	if d.Site.DNS != nil {
		if ok, msg := d.Site.DNS.CheckHost(r.Context(), domain); !ok {
			writeError(w, reqID, APIError{Status: http.StatusBadRequest, Code: "dns_not_pointing", Message: msg})
			return
		}
	}

	// Account, database and database user share one name: zx_<label>.
	name := "zx_" + strings.ReplaceAll(label, "-", "_")
	siteID, jobID, existed, err := d.Sites.CreateSiteWithJob(r.Context(), store.NewSite{
		OwnerID:        user.ID,
		NodeID:         d.Site.NodeID,
		ZoneID:         zoneID,
		Slug:           label,
		Domain:         domain,
		LinuxUser:      name,
		DBName:         name,
		DBUser:         name,
		PHPVersion:     d.Site.PHPVersion,
		IdempotencyKey: key,
		JobType:        provision.JobType,
	})
	if errors.Is(err, store.ErrConflict) {
		d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, Action: "site.create", TargetType: "domain", TargetID: domain, Result: "denied", ErrorCode: ErrAlreadyExists.Code})
		writeError(w, reqID, ErrAlreadyExists)
		return
	}
	if err != nil {
		d.internal(w, r, "sites.create", err)
		return
	}
	site, err := d.Sites.GetSite(r.Context(), siteID)
	if err != nil {
		d.internal(w, r, "sites.create", err)
		return
	}
	if !existed {
		d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.create", TargetType: "site", TargetID: siteID, Result: "success"})
		d.Site.StartJob(jobID)
		writeJSON(w, http.StatusAccepted, siteResponse{Site: site, JobID: jobID})
		return
	}
	writeJSON(w, http.StatusOK, siteResponse{Site: site, JobID: jobID})
}

// siteDetail is what the site page shows: the site, its latest job and steps.
type siteDetail struct {
	Site  store.Site      `json:"site"`
	Job   *store.Job      `json:"job,omitempty"`
	Steps []store.JobStep `json:"steps,omitempty"`
}

func (d Deps) loadSiteForUser(w http.ResponseWriter, r *http.Request) (store.Site, *store.User, bool) {
	user, _ := currentUser(r)
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return store.Site{}, nil, false
	}
	site, err := d.Sites.GetSite(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !canAccess(user, site.OwnerID)) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return store.Site{}, nil, false
	}
	if err != nil {
		d.internal(w, r, "sites.get", err)
		return store.Site{}, nil, false
	}
	return site, user, true
}

func (d Deps) handleGetSite(w http.ResponseWriter, r *http.Request) {
	site, _, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	detail := siteDetail{Site: site}
	if job, err := d.Sites.LatestJobForSite(r.Context(), site.ID); err == nil {
		detail.Job = &job
		if steps, err := d.Sites.JobSteps(r.Context(), job.ID); err == nil {
			detail.Steps = steps
		}
	}
	writeJSON(w, http.StatusOK, detail)
}

type siteCredentials struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleSiteCredentials shows the WordPress administrator login. The password is
// derived on demand from the server secret, so it is never stored.
func (d Deps) handleSiteCredentials(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	if site.State != "ready" {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusConflict, Code: "site_not_ready", Message: "the website is not ready yet"})
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.credentials.view", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusOK, siteCredentials{
		URL:      "http://" + site.Domain + "/wp-admin/",
		Username: "zenexadmin",
		Password: provision.WPAdminPassword(d.Site.SecretKey, site.ID),
	})
}

// ---------------------------------------------------------------------------
// Jobs
// ---------------------------------------------------------------------------

type jobDetail struct {
	Job   store.Job       `json:"job"`
	Steps []store.JobStep `json:"steps"`
}

func (d Deps) loadJobForUser(w http.ResponseWriter, r *http.Request) (store.Job, *store.User, bool) {
	user, _ := currentUser(r)
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return store.Job{}, nil, false
	}
	job, err := d.Sites.GetJob(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !canAccess(user, job.ActorID)) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return store.Job{}, nil, false
	}
	if err != nil {
		d.internal(w, r, "jobs.get", err)
		return store.Job{}, nil, false
	}
	return job, user, true
}

func (d Deps) handleGetJob(w http.ResponseWriter, r *http.Request) {
	job, _, ok := d.loadJobForUser(w, r)
	if !ok {
		return
	}
	steps, err := d.Sites.JobSteps(r.Context(), job.ID)
	if err != nil {
		d.internal(w, r, "jobs.get", err)
		return
	}
	writeJSON(w, http.StatusOK, jobDetail{Job: job, Steps: steps})
}

func (d Deps) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	job, user, ok := d.loadJobForUser(w, r)
	if !ok {
		return
	}
	requeued, err := d.Sites.RequeueFailedJob(r.Context(), job.ID)
	if err != nil {
		d.internal(w, r, "jobs.retry", err)
		return
	}
	if !requeued {
		writeError(w, requestIDFrom(r), ErrNotRetryable)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "job.retry", TargetType: "job", TargetID: job.ID, Result: "success"})
	d.Site.StartJob(job.ID)
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": job.ID, "status": "queued"})
}

// internal logs the cause and returns a generic error to the client.
func (d Deps) internal(w http.ResponseWriter, r *http.Request, op string, err error) {
	d.Log.Error("request failed", "request_id", requestIDFrom(r), "operation", op, "error_code", ErrInternal.Code, "error", err)
	writeError(w, requestIDFrom(r), ErrInternal)
}
