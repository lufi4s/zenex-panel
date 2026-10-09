package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/packages/validation"
)

const (
	siteUUID    = "33333333-3333-3333-3333-333333333333"
	cpanelScan  = "/api/v1/migrations/cpanel/scan"
	cpanelStart = "/api/v1/migrations/cpanel"
)

// stubLookup replaces the host name lookup for one test.
func stubLookup(t *testing.T, ips map[string][]net.IP) {
	t.Helper()
	old := lookupIPs
	lookupIPs = func(_ context.Context, host string) ([]net.IP, error) {
		if v, ok := ips[host]; ok {
			return v, nil
		}
		return nil, errors.New("no such host")
	}
	t.Cleanup(func() { lookupIPs = old })
}

func TestCpanelCredsValidation(t *testing.T) {
	stubLookup(t, map[string][]net.IP{
		"cpanel.example.com": {net.ParseIP("198.51.100.5")},
		"sneaky.example.com": {net.ParseIP("127.0.0.1")},
	})
	good := cpanelConn{Host: "cpanel.example.com", Username: "acct", Password: "pw"}
	creds, bad := cpanelCreds(context.Background(), good)
	if bad != nil || creds.Port != 22 || creds.Host != "cpanel.example.com" {
		t.Fatalf("good input refused: %+v %v", creds, bad)
	}

	cases := map[string]func(c *cpanelConn){
		"unknown host":    func(c *cpanelConn) { c.Host = "nowhere.example.com" },
		"loopback name":   func(c *cpanelConn) { c.Host = "sneaky.example.com" },
		"loopback ip":     func(c *cpanelConn) { c.Host = "127.0.0.1" },
		"link-local ip":   func(c *cpanelConn) { c.Host = "169.254.169.254" },
		"unspecified":     func(c *cpanelConn) { c.Host = "0.0.0.0" },
		"host with space": func(c *cpanelConn) { c.Host = "bad host" },
		"port too big":    func(c *cpanelConn) { c.Port = 70000 },
		"upper username":  func(c *cpanelConn) { c.Username = "Acct;rm" },
		"empty password":  func(c *cpanelConn) { c.Password = "" },
		"newline in pw":   func(c *cpanelConn) { c.Password = "a\nb" },
	}
	for name, mutate := range cases {
		c := good
		mutate(&c)
		if _, bad := cpanelCreds(context.Background(), c); bad == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestMigrationSlugAlwaysPassesTheSiteRules(t *testing.T) {
	cases := map[string]string{
		"www.example.com": "www-example-com",
		"example.com":     "example-com",
		"a--b.example.io": "a-b-example-io",
	}
	for domain, want := range cases {
		if got := migrationSlug(domain); got != want {
			t.Errorf("migrationSlug(%q) = %q, want %q", domain, got, want)
		}
	}
	for _, domain := range []string{
		"123.example.com", "x.y", "very-long-subdomain-name.another-long-label.example.org",
		"-.com", "admin.example.com", "ü.example.com",
	} {
		slug := migrationSlug(domain)
		if err := validation.SiteSlug(slug); err != nil {
			t.Errorf("migrationSlug(%q) = %q fails the site rules: %v", domain, slug, err)
		}
		if len("zx_"+strings.ReplaceAll(slug, "-", "_")) > 32 {
			t.Errorf("the account name for %q is longer than 32 characters", domain)
		}
	}
	if migrationSlug("very-long-subdomain-name.another-long-label.example.org") == migrationSlug("very-long-subdomain-name.another-long-label.example.net") {
		t.Error("two long domains share a slug")
	}
}

func TestMatchDomainPrefersTheLongestMatch(t *testing.T) {
	domains := []store.Domain{{Apex: "example.com"}, {Apex: "shop.example.com"}}
	if d, ok := matchDomain(domains, "www.shop.example.com"); !ok || d.Apex != "shop.example.com" {
		t.Fatalf("match = %+v %v", d, ok)
	}
	if d, ok := matchDomain(domains, "example.com"); !ok || d.Apex != "example.com" {
		t.Fatalf("apex match = %+v %v", d, ok)
	}
	if _, ok := matchDomain(domains, "notexample.com"); ok {
		t.Fatal("a name that only ends with the same letters matched")
	}
}

// migrationSites adds domains and site creation to the progress fakes.
type migrationSites struct {
	*progressSites
	domains  []store.Domain
	created  []store.NewSite
	conflict bool
}

func (m *migrationSites) ListDomains(context.Context, string) ([]store.Domain, error) {
	return m.domains, nil
}

func (m *migrationSites) DomainOwnedBy(_ context.Context, _, apex string) (string, error) {
	for _, d := range m.domains {
		if d.Apex == apex {
			return "zone-1", nil
		}
	}
	return "", store.ErrNotFound
}

func (m *migrationSites) CreateSiteWithJob(_ context.Context, in store.NewSite) (string, string, bool, error) {
	if m.conflict {
		return "", "", false, store.ErrConflict
	}
	m.created = append(m.created, in)
	return siteUUID, "build-job", false, nil
}

// migrationManage records scans and migrations.
type migrationManage struct {
	settingsManage
	installs []manage.CpanelInstall
	scanErr  error
	req      *manage.MigrationRequest
}

func (m *migrationManage) ScanCpanel(context.Context, manage.CpanelCreds) ([]manage.CpanelInstall, error) {
	return m.installs, m.scanErr
}

func (m *migrationManage) StartMigration(_ context.Context, req manage.MigrationRequest) (string, error) {
	m.req = &req
	return "migrate-job", nil
}

type migrationEnv struct {
	settingsEnv
	sites   *migrationSites
	manage  *migrationManage
	started []string
}

func newMigrationEnv(t *testing.T, domains []store.Domain) *migrationEnv {
	t.Helper()
	stubLookup(t, map[string][]net.IP{"cpanel.example.com": {net.ParseIP("198.51.100.5")}})
	ps := &progressSites{}
	e := newProgressEnv(t, ps)
	e.sites.siteState = "provisioning"
	env := &migrationEnv{settingsEnv: e, sites: &migrationSites{progressSites: ps, domains: domains}, manage: &migrationManage{}}
	env.h = NewRouter(Deps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:  e.fs,
		Sites:  env.sites,
		Manage: env.manage,
		Alerts: e.alert,
		Site: SiteSettings{
			NodeID: "node-1", PHPVersion: "8.3", SecretKey: []byte(testServerSecret), PublicIP: "203.0.113.7",
			StartJob: func(id string) { env.started = append(env.started, id) },
		},
	})
	return env
}

const scanBody = `{"host":"cpanel.example.com","username":"acct","password":"pw"}`

const startBody = `{"host":"cpanel.example.com","username":"acct","password":"pw","path":"/home/acct/public_html","domain":"www.example.com"}`

func TestMigrationEndpointsAreAdminOnly(t *testing.T) {
	e := newMigrationEnv(t, nil)
	customer := e.login(t, "customer@example.com")
	for _, path := range []string{cpanelScan, cpanelStart} {
		rec := e.call(t, http.MethodPost, path, startBody, customer, true)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s as customer = %d, want 403", path, rec.Code)
		}
	}
	rec := e.call(t, http.MethodPost, cpanelScan, startBody, nil, true)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("signed out = %d, want 401", rec.Code)
	}
}

func TestScanCpanelMarksWhatCanBeMigrated(t *testing.T) {
	e := newMigrationEnv(t, []store.Domain{{Apex: "example.com"}})
	e.manage.installs = []manage.CpanelInstall{
		{Path: "/home/acct/public_html", Domain: "www.example.com", SizeKB: 5},
		{Path: "/home/acct/other", Domain: "unrelated.org"},
		{Path: "/home/acct/unknown"},
	}
	admin := e.login(t, testEmail)

	rec := e.call(t, http.MethodPost, cpanelScan, scanBody, admin, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Installs []struct {
			Path          string `json:"path"`
			MatchedDomain string `json:"matched_domain"`
		} `json:"installs"`
		ServerIP string `json:"server_ip"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Installs) != 3 || body.Installs[0].MatchedDomain != "example.com" || body.Installs[1].MatchedDomain != "" {
		t.Fatalf("installs = %+v", body.Installs)
	}
	if body.ServerIP != "203.0.113.7" {
		t.Fatalf("server_ip = %q", body.ServerIP)
	}
}

func TestScanCpanelShowsTheHelperMessage(t *testing.T) {
	e := newMigrationEnv(t, nil)
	e.manage.scanErr = errors.New("authentication failed")
	admin := e.login(t, testEmail)
	rec := e.call(t, http.MethodPost, cpanelScan, scanBody, admin, true)
	if rec.Code == http.StatusOK {
		t.Fatal("a failed scan returned 200")
	}
	if strings.Contains(rec.Body.String(), "pw") && strings.Contains(rec.Body.String(), `"password"`) {
		t.Fatalf("the response echoes the request: %s", rec.Body.String())
	}
}

func TestMigrateCreatesTheWebsiteAndStartsTheMigration(t *testing.T) {
	e := newMigrationEnv(t, []store.Domain{{Apex: "example.com"}})
	admin := e.login(t, testEmail)

	rec := e.call(t, http.MethodPost, cpanelStart, startBody, admin, true)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(e.sites.created) != 1 {
		t.Fatalf("sites created = %d", len(e.sites.created))
	}
	got := e.sites.created[0]
	if got.Domain != "www.example.com" || got.Slug != "www-example-com" || got.LinuxUser != "zx_www_example_com" || got.DBName != got.LinuxUser || got.ZoneID != "zone-1" {
		t.Fatalf("new site = %+v", got)
	}
	if len(e.started) != 1 || e.started[0] != "build-job" {
		t.Fatalf("started builds = %v", e.started)
	}
	req := e.manage.req
	if req == nil || req.ProvisionJobID != "build-job" || req.SourceDomain != "www.example.com" || req.SourcePath != "/home/acct/public_html" || req.Creds.Password != "pw" {
		t.Fatalf("migration request = %+v", req)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if string(body["job_id"]) != `"migrate-job"` {
		t.Fatalf("job_id = %s", body["job_id"])
	}
	if strings.Contains(rec.Body.String(), `"pw"`) {
		t.Fatal("the response carries the cPanel password")
	}
}

func TestMigrateNeedsTheDomainToBeConnected(t *testing.T) {
	e := newMigrationEnv(t, nil)
	admin := e.login(t, testEmail)
	rec := e.call(t, http.MethodPost, cpanelStart, startBody, admin, true)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "domain_not_connected" {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(e.sites.created) != 0 || e.manage.req != nil {
		t.Fatal("something was created for an unconnected domain")
	}
}

func TestMigrateReportsAnExistingWebsite(t *testing.T) {
	e := newMigrationEnv(t, []store.Domain{{Apex: "example.com"}})
	e.sites.conflict = true
	admin := e.login(t, testEmail)
	rec := e.call(t, http.MethodPost, cpanelStart, startBody, admin, true)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "site_exists" {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if e.manage.req != nil {
		t.Fatal("a migration started for a conflicting website")
	}
}

func TestMigrateRetryUsesTheExistingWebsite(t *testing.T) {
	e := newMigrationEnv(t, nil)
	e.sites.settingsSites.siteState = "ready"
	admin := e.login(t, testEmail)
	body := `{"host":"cpanel.example.com","username":"acct","password":"pw","path":"/home/acct/public_html","site_id":"` + siteUUID + `","source_domain":"old.example.org"}`

	rec := e.call(t, http.MethodPost, cpanelStart, body, admin, true)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(e.sites.created) != 0 || len(e.started) != 0 {
		t.Fatal("a retry built another website")
	}
	if req := e.manage.req; req == nil || req.ProvisionJobID != "" || req.SourceDomain != "old.example.org" {
		t.Fatalf("migration request = %+v", req)
	}
}
