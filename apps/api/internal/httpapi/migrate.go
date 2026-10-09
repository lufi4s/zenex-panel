package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/provision"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/packages/validation"
)

var (
	cpanelHostRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
	cpanelUserRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
)

// lookupIPs resolves a host name. It is a variable so tests do not use the network.
var lookupIPs = func(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

// cpanelConn is the sign-in to the cPanel account, sent with every migration request.
type cpanelConn struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type cpanelMigrateRequest struct {
	cpanelConn
	// Path is the website folder on the cPanel account, as the scan listed it.
	Path string `json:"path"`
	// Domain is the address the website gets on this panel. It must belong to a domain that
	// is connected under Domains.
	Domain string `json:"domain"`
	// SourceDomain is the address the website has on cPanel. It defaults to Domain.
	SourceDomain string `json:"source_domain"`
	// SiteID retries a migration into a website that already exists here.
	SiteID string `json:"site_id"`
}

func badRequest(code, message string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: code, Message: message}
}

// cpanelCreds checks the sign-in details. The host must be another machine: a name that points
// at this server or at a link-local address is refused, so the panel cannot be used to reach
// its own services.
func cpanelCreds(ctx context.Context, in cpanelConn) (manage.CpanelCreds, *APIError) {
	host := strings.TrimSpace(in.Host)
	port := in.Port
	if port == 0 {
		port = 22
	}
	username := strings.TrimSpace(in.Username)
	switch {
	case host == "" || (!cpanelHostRe.MatchString(host) && net.ParseIP(host) == nil):
		return manage.CpanelCreds{}, badRequest("invalid_host", "Enter the cPanel server's host name or IP address.")
	case port < 1 || port > 65535:
		return manage.CpanelCreds{}, badRequest("invalid_port", "The SSH port must be between 1 and 65535.")
	case !cpanelUserRe.MatchString(username):
		return manage.CpanelCreds{}, badRequest("invalid_username", "Enter the cPanel username.")
	case in.Password == "" || len(in.Password) > 512 || strings.ContainsAny(in.Password, "\x00\r\n"):
		return manage.CpanelCreds{}, badRequest("invalid_password", "Enter the cPanel password.")
	}
	if err := checkRemoteHost(ctx, host); err != nil {
		return manage.CpanelCreds{}, badRequest("invalid_host", err.Error())
	}
	return manage.CpanelCreds{Host: host, Port: port, Username: username, Password: in.Password}, nil
}

// checkRemoteHost refuses hosts that resolve to this machine or to link-local addresses.
func checkRemoteHost(ctx context.Context, host string) error {
	ips := []net.IP{}
	if ip := net.ParseIP(host); ip != nil {
		ips = append(ips, ip)
	} else {
		lctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		found, err := lookupIPs(lctx, host)
		if err != nil || len(found) == 0 {
			return errors.New("The host name could not be found. Check it and try again.")
		}
		ips = found
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return errors.New("That address is not a different server. Enter the cPanel server's own host name or IP address.")
		}
	}
	return nil
}

// matchDomain finds the connected domain that covers host: the host is the domain itself or one
// of its subdomains. The longest match wins.
func matchDomain(domains []store.Domain, host string) (store.Domain, bool) {
	var best store.Domain
	found := false
	for _, d := range domains {
		if host == d.Apex || strings.HasSuffix(host, "."+d.Apex) {
			if !found || len(d.Apex) > len(best.Apex) {
				best, found = d, true
			}
		}
	}
	return best, found
}

// migrationSlug makes the site name from the whole domain, so two websites called "www" on
// different domains do not collide. It always passes the site slug rules.
func migrationSlug(domain string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(domain) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case b.Len() > 0 && !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" || slug[0] < 'a' || slug[0] > 'z' {
		slug = "site-" + slug
	}
	if len(slug) > 28 {
		sum := sha256.Sum256([]byte(domain))
		slug = strings.TrimRight(slug[:23], "-") + "-" + hex.EncodeToString(sum[:2])
	}
	if validation.SiteSlug(slug) != nil {
		sum := sha256.Sum256([]byte(domain))
		slug = "m-" + hex.EncodeToString(sum[:4])
	}
	return slug
}

// cpanelInstallView is one install of the scan, with what the panel knows about its domain.
type cpanelInstallView struct {
	manage.CpanelInstall
	// MatchedDomain is the connected domain that covers the install's address, or "".
	MatchedDomain string `json:"matched_domain"`
	// ExistingSiteID is the website that already uses this address here, or "".
	ExistingSiteID string `json:"existing_site_id"`
}

// handleScanCpanel lists the WordPress installs on a cPanel account. It only reads from cPanel.
func (d Deps) handleScanCpanel(w http.ResponseWriter, r *http.Request) {
	user, ok := d.requireAdminUser(w, r)
	if !ok {
		return
	}
	var in cpanelConn
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	creds, bad := cpanelCreds(r.Context(), in)
	if bad != nil {
		writeError(w, requestIDFrom(r), *bad)
		return
	}

	// An account with many websites takes minutes to scan, longer than the server's usual write limit.
	allowSlowRequest(w, 0, 6*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	installs, err := d.Manage.ScanCpanel(ctx, creds)
	if err != nil {
		d.manageError(w, r, "migrations.cpanel.scan", err)
		return
	}

	domains, err := d.Sites.ListDomains(ctx, user.ID)
	if err != nil {
		d.internal(w, r, "migrations.cpanel.scan.domains", err)
		return
	}
	sites, err := d.Sites.ListSites(ctx, "")
	if err != nil {
		d.internal(w, r, "migrations.cpanel.scan.sites", err)
		return
	}
	byDomain := make(map[string]string, len(sites))
	for _, s := range sites {
		byDomain[s.Domain] = s.ID
	}
	views := make([]cpanelInstallView, 0, len(installs))
	for _, in := range installs {
		v := cpanelInstallView{CpanelInstall: in}
		if in.Domain != "" {
			if dom, found := matchDomain(domains, in.Domain); found {
				v.MatchedDomain = dom.Apex
			}
			v.ExistingSiteID = byDomain[in.Domain]
		}
		views = append(views, v)
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "migration.cpanel.scan", TargetType: "user", TargetID: user.ID, Result: "success"})
	writeJSON(w, http.StatusOK, map[string]any{"installs": views, "server_ip": d.Site.PublicIP})
}

// handleMigrateCpanel builds a website here and fills it from a cPanel website. Nothing on the
// cPanel account is changed. The DNS of the domain is not touched either: the person switches it
// when the copy has been checked.
func (d Deps) handleMigrateCpanel(w http.ResponseWriter, r *http.Request) {
	user, ok := d.requireAdminUser(w, r)
	if !ok {
		return
	}
	reqID := requestIDFrom(r)
	var in cpanelMigrateRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, reqID, ErrInvalidRequest)
		return
	}
	creds, bad := cpanelCreds(r.Context(), in.cpanelConn)
	if bad != nil {
		writeError(w, reqID, *bad)
		return
	}
	srcPath := strings.TrimSpace(in.Path)
	if !strings.HasPrefix(srcPath, "/") || len(srcPath) > 255 || strings.ContainsAny(srcPath, "\x00\r\n") {
		writeError(w, reqID, *badRequest("invalid_path", "Choose a website from the list."))
		return
	}
	if d.Site.NodeID == "" {
		writeError(w, reqID, ErrNodeMissing)
		return
	}

	site, provisionJob, apiErr := d.migrationTarget(r, user, in)
	if apiErr != nil {
		writeError(w, reqID, *apiErr)
		return
	}
	// The address on cPanel: given, else the address asked for here, else the existing website's.
	source := strings.ToLower(strings.TrimSpace(in.SourceDomain))
	if source == "" {
		source = strings.ToLower(strings.TrimSpace(in.Domain))
	}
	if source == "" {
		source = site.Domain
	}
	if validation.DomainName(source) != nil {
		writeError(w, reqID, *badRequest("invalid_domain", "The address on cPanel is not a valid domain."))
		return
	}

	jobID, err := d.Manage.StartMigration(r.Context(), manage.MigrationRequest{
		Site: site, ProvisionJobID: provisionJob, Creds: creds,
		SourcePath: srcPath, SourceDomain: source, ActorID: user.ID,
	})
	if err != nil {
		d.manageError(w, r, "migrations.cpanel.start", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "migration.cpanel.start", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusAccepted, map[string]any{
		"site": site, "job_id": jobID, "provision_job_id": provisionJob, "server_ip": d.Site.PublicIP,
	})
}

// migrationTarget returns the website that receives the migration. For a retry it is the
// existing website; otherwise a new one is created here and its build job is started. The
// build job ID is empty for an existing website.
func (d Deps) migrationTarget(r *http.Request, user *store.User, in cpanelMigrateRequest) (store.Site, string, *APIError) {
	ctx := r.Context()
	if in.SiteID != "" {
		if !uuidRe.MatchString(in.SiteID) {
			return store.Site{}, "", &ErrNotFound
		}
		site, err := d.Sites.GetSite(ctx, in.SiteID)
		if errors.Is(err, store.ErrNotFound) {
			return store.Site{}, "", &ErrNotFound
		}
		if err != nil {
			return store.Site{}, "", d.failure(r, "migrations.cpanel.site", err)
		}
		if site.State != "ready" {
			return store.Site{}, "", &APIError{Status: http.StatusConflict, Code: "site_not_ready", Message: "The website is not ready yet."}
		}
		busy, err := d.siteBusyWithBackupWork(ctx, site.ID)
		if err != nil {
			return store.Site{}, "", d.failure(r, "migrations.cpanel.busy", err)
		}
		if busy {
			return store.Site{}, "", &errBackupBusy
		}
		return site, "", nil
	}

	domain := strings.ToLower(strings.TrimSpace(in.Domain))
	if validation.DomainName(domain) != nil {
		return store.Site{}, "", badRequest("invalid_domain", "Enter the website's address, for example www.example.com.")
	}
	domains, err := d.Sites.ListDomains(ctx, user.ID)
	if err != nil {
		return store.Site{}, "", d.failure(r, "migrations.cpanel.domains", err)
	}
	apex, found := matchDomain(domains, domain)
	if !found {
		return store.Site{}, "", &APIError{Status: http.StatusBadRequest, Code: "domain_not_connected",
			Message: "Add the domain of " + domain + " under Domains first, then try again. Its DNS can stay on cPanel until you switch."}
	}
	zoneID, err := d.Sites.DomainOwnedBy(ctx, user.ID, apex.Apex)
	if err != nil {
		return store.Site{}, "", d.failure(r, "migrations.cpanel.zone", err)
	}
	phpVersion, err := d.defaultPHPVersion(ctx)
	if err != nil {
		return store.Site{}, "", d.failure(r, "migrations.cpanel.php", err)
	}

	slug := migrationSlug(domain)
	name := "zx_" + strings.ReplaceAll(slug, "-", "_")
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return store.Site{}, "", d.failure(r, "migrations.cpanel.key", err)
	}
	// The domain's DNS still points at cPanel, so it is not checked here. The wildcard check
	// on the connected domain is for new websites built on this server.
	siteID, jobID, _, err := d.Sites.CreateSiteWithJob(ctx, store.NewSite{
		OwnerID: user.ID, NodeID: d.Site.NodeID, ZoneID: zoneID, Slug: slug, Domain: domain,
		LinuxUser: name, DBName: name, DBUser: name, PHPVersion: phpVersion,
		IdempotencyKey: "migrate-" + hex.EncodeToString(key), JobType: provision.JobType,
	})
	if errors.Is(err, store.ErrConflict) {
		return store.Site{}, "", &APIError{Status: http.StatusConflict, Code: "site_exists",
			Message: "A website for " + domain + " already exists here. Open it, or delete it and try again."}
	}
	if err != nil {
		return store.Site{}, "", d.failure(r, "migrations.cpanel.create", err)
	}
	site, err := d.Sites.GetSite(ctx, siteID)
	if err != nil {
		return store.Site{}, "", d.failure(r, "migrations.cpanel.get", err)
	}
	d.Site.StartJob(jobID)
	return site, jobID, nil
}

// failure logs an unexpected error and returns the generic API error for it.
func (d Deps) failure(r *http.Request, op string, err error) *APIError {
	d.Log.Error("request failed", "op", op, "error", err, "request_id", requestIDFrom(r))
	return &ErrInternal
}
