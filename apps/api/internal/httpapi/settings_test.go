package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/alerts"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/auth"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// settingsSites implements the settings and site methods the tests use. Any other
// SiteStore method is nil and panics, which the router reports as a 500.
type settingsSites struct {
	SiteStore
	defaults   store.SiteDefaults
	backup     store.BackupSettings
	sftpKey    string
	sealed     string
	created    *store.NewSite
	autoUpdate map[string]bool
	siteState  string
}

func (s *settingsSites) GetSealedSFTPPassword(context.Context) (string, error) {
	if s.sealed == "" {
		return "", store.ErrNotFound
	}
	return s.sealed, nil
}
func (s *settingsSites) SetSealedSFTPPassword(_ context.Context, _ string, sealed string) error {
	s.sealed = sealed
	return nil
}

func (s *settingsSites) GetSiteDefaults(context.Context) (store.SiteDefaults, error) {
	return s.defaults, nil
}
func (s *settingsSites) SetSiteDefaults(_ context.Context, _ string, v store.SiteDefaults) error {
	s.defaults = v
	return nil
}
func (s *settingsSites) GetBackupSettings(context.Context) (store.BackupSettings, error) {
	return s.backup, nil
}
func (s *settingsSites) SetBackupSettings(_ context.Context, _ string, v store.BackupSettings) error {
	s.backup = v
	return nil
}
func (s *settingsSites) GetSite(_ context.Context, id string) (store.Site, error) {
	return store.Site{ID: id, State: s.siteState, OwnerID: "11111111-1111-1111-1111-111111111111", Domain: "shop.example.com", LinuxUser: "zx_shop"}, nil
}
func (s *settingsSites) SetSiteAutoUpdate(_ context.Context, id string, on bool) error {
	if s.autoUpdate == nil {
		s.autoUpdate = map[string]bool{}
	}
	s.autoUpdate[id] = on
	return nil
}
func (s *settingsSites) FindSiteByIdempotencyKey(context.Context, string, string) (string, string, error) {
	return "", "", store.ErrNotFound
}
func (s *settingsSites) DomainOwnedBy(context.Context, string, string) (string, error) {
	return "22222222-2222-2222-2222-222222222222", nil
}
func (s *settingsSites) CreateSiteWithJob(_ context.Context, in store.NewSite) (string, string, bool, error) {
	s.created = &in
	return "33333333-3333-3333-3333-333333333333", "44444444-4444-4444-4444-444444444444", false, nil
}

// settingsManage implements the PHP version list and the maintenance call.
type settingsManage struct {
	SiteManager
	installed   []string
	maintenance *bool
	tested      []store.SFTPDestination
	testErr     error
}

func (m *settingsManage) PHPVersions(context.Context) ([]string, error) {
	return m.installed, nil
}
func (m *settingsManage) SetMaintenance(_ context.Context, _ store.Site, on bool) error {
	m.maintenance = &on
	return nil
}
func (m *settingsManage) StartBackup(context.Context, store.Site, string) (string, error) {
	return "job-9", nil
}

// fakeAlerts validates like the real service but keeps nothing.
type fakeAlerts struct {
	saved    *alerts.Input
	testErr  error
	testSent int
}

func (f *fakeAlerts) SendTestEmail(context.Context) error {
	f.testSent++
	return f.testErr
}

func (f *fakeAlerts) View(context.Context) (alerts.View, error) {
	return alerts.View{Thresholds: alerts.Thresholds{CPU: 85, Memory: 90, Disk: 90}}, nil
}
func (f *fakeAlerts) Update(_ context.Context, _ string, in alerts.Input) error {
	if _, err := alerts.Validate(alerts.Settings{Email: in.Email.EmailSettings, Telegram: in.Telegram.TelegramSettings, Thresholds: in.Thresholds}); err != nil {
		return err
	}
	f.saved = &in
	return nil
}

type settingsEnv struct {
	h      http.Handler
	fs     *fakeStore
	sites  *settingsSites
	manage *settingsManage
	alert  *fakeAlerts
}

// testServerSecret stands in for ZENEX_SECRET_KEY in settings tests.
const testServerSecret = "0123456789abcdef0123456789abcdef-test"

func newSettingsEnv(t *testing.T) settingsEnv {
	t.Helper()
	return newSettingsEnvWithSecret(t, testServerSecret)
}

func newSettingsEnvWithSecret(t *testing.T, secret string) settingsEnv {
	t.Helper()
	fs := newFakeStore(t, testEmail, testPassword)
	hash, err := auth.HashPassword(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	fs.users["customer@example.com"] = &store.User{
		ID: "55555555-5555-5555-5555-555555555555", Email: "customer@example.com",
		PasswordHash: hash, Status: "active", Roles: []string{"customer"},
	}
	env := settingsEnv{
		fs:     fs,
		sites:  &settingsSites{siteState: "ready", backup: store.DefaultBackupSettings()},
		manage: &settingsManage{installed: []string{"8.3", "8.2"}},
		alert:  &fakeAlerts{},
	}
	env.h = NewRouter(Deps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:  fs,
		Sites:  env.sites,
		Manage: env.manage,
		Alerts: env.alert,
		Site:   SiteSettings{NodeID: "node-1", PHPVersion: "8.3", SecretKey: []byte(secret), StartJob: func(string) {}},
	})
	return env
}

// call sends a request. Mutations carry the CSRF header and the session cookie
// unless the caller says otherwise.
func (e settingsEnv) call(t *testing.T, method, path, body string, cookie *http.Cookie, csrf bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf {
		req.Header.Set("X-Requested-With", "zenex")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e settingsEnv) login(t *testing.T, email string) *http.Cookie {
	t.Helper()
	rec := postLogin(e.h, `{"email":"`+email+`","password":"`+testPassword+`"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("login as %s: %d %s", email, rec.Code, rec.Body.String())
	}
	return sessionCookieFrom(rec)
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("not an error body: %s", rec.Body.String())
	}
	return env.Error.Code
}

func TestSiteDefaultsFallBackToServerPHPVersion(t *testing.T) {
	e := newSettingsEnv(t)
	cookie := e.login(t, testEmail)
	rec := e.call(t, http.MethodGet, "/api/v1/settings/defaults", "", cookie, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"php_version":"8.3"`) {
		t.Fatalf("GET defaults = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSiteDefaultsRequireSignInForReading(t *testing.T) {
	e := newSettingsEnv(t)
	rec := e.call(t, http.MethodGet, "/api/v1/settings/defaults", "", nil, false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestSiteDefaultsWriteNeedsCSRFAndAdmin(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	customer := e.login(t, "customer@example.com")
	body := `{"php_version":"8.2"}`

	if rec := e.call(t, http.MethodPut, "/api/v1/settings/defaults", body, admin, false); rec.Code != http.StatusForbidden {
		t.Fatalf("PUT without CSRF = %d, want 403", rec.Code)
	}
	if rec := e.call(t, http.MethodPut, "/api/v1/settings/defaults", body, customer, true); rec.Code != http.StatusForbidden {
		t.Fatalf("PUT as customer = %d, want 403", rec.Code)
	}
	if e.sites.defaults.PHPVersion != "" {
		t.Fatal("refused write changed the default")
	}
}

func TestSiteDefaultsRejectUninstalledVersion(t *testing.T) {
	e := newSettingsEnv(t)
	cookie := e.login(t, testEmail)
	rec := e.call(t, http.MethodPut, "/api/v1/settings/defaults", `{"php_version":"7.4"}`, cookie, true)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "php_not_installed" {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestSiteDefaultsSaveInstalledVersion(t *testing.T) {
	e := newSettingsEnv(t)
	cookie := e.login(t, testEmail)
	rec := e.call(t, http.MethodPut, "/api/v1/settings/defaults", `{"php_version":"8.2"}`, cookie, true)
	if rec.Code != http.StatusOK || e.sites.defaults.PHPVersion != "8.2" {
		t.Fatalf("status %d, stored %+v", rec.Code, e.sites.defaults)
	}
}

func TestNewSitesUseSavedDefault(t *testing.T) {
	e := newSettingsEnv(t)
	cookie := e.login(t, testEmail)
	e.sites.defaults = store.SiteDefaults{PHPVersion: "8.2"}
	body := `{"label":"shop","apex":"example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sites", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "zenex")
	req.Header.Set("Idempotency-Key", "create-shop-0001")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	if e.sites.created == nil || e.sites.created.PHPVersion != "8.2" {
		t.Fatalf("site created with %+v, want the saved 8.2", e.sites.created)
	}
}

func TestBackupSettingsValidation(t *testing.T) {
	cases := map[string]store.BackupSettings{
		"hour too high":     {ScheduleHour: 24, RetentionDays: 7},
		"hour negative":     {ScheduleHour: -1, RetentionDays: 7},
		"retention zero":    {ScheduleHour: 3, RetentionDays: 0},
		"retention too big": {ScheduleHour: 3, RetentionDays: 91},
	}
	for name, in := range cases {
		if _, problem := validBackupSettings(in); problem == "" {
			t.Errorf("%s: accepted %+v", name, in)
		}
	}
	if _, problem := validBackupSettings(store.DefaultBackupSettings()); problem != "" {
		t.Errorf("defaults rejected: %s", problem)
	}
}

func TestBackupSettingsEndpointsAreAdminOnly(t *testing.T) {
	e := newSettingsEnv(t)
	customer := e.login(t, "customer@example.com")
	if rec := e.call(t, http.MethodGet, "/api/v1/settings/backups", "", customer, false); rec.Code != http.StatusForbidden {
		t.Fatalf("GET as customer = %d", rec.Code)
	}
	admin := e.login(t, testEmail)
	rec := e.call(t, http.MethodPut, "/api/v1/settings/backups", `{"schedule_hour":24,"retention_days":7}`, admin, true)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_backup_settings" {
		t.Fatalf("invalid hour = %d %s", rec.Code, rec.Body.String())
	}
	rec = e.call(t, http.MethodPut, "/api/v1/settings/backups", `{"schedule_hour":2,"retention_days":30}`, admin, true)
	if rec.Code != http.StatusOK || e.sites.backup.ScheduleHour != 2 || e.sites.backup.RetentionDays != 30 || e.sites.backup.Destination.Type != store.BackupDestLocal {
		t.Fatalf("valid save = %d, stored %+v", rec.Code, e.sites.backup)
	}
}

func TestAlertSettingsRejectInvalidInput(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	body := `{"email":{"enabled":true,"host":"smtp.example.com","port":587,"from":"alerts@example.com","to":""},
		"telegram":{"enabled":false,"chat_id":""},"thresholds":{"cpu":85,"memory":90,"disk":90}}`
	rec := e.call(t, http.MethodPut, "/api/v1/settings/alerts", body, admin, true)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_alerts" {
		t.Fatalf("empty recipient = %d %s", rec.Code, rec.Body.String())
	}
	if e.alert.saved != nil {
		t.Fatal("invalid configuration was saved")
	}
}

func TestAlertSettingsAcceptAGetResponseSentBack(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	body := `{"email":{"enabled":false,"host":"","port":587,"username":"","from":"","to":"","password_set":true},
		"telegram":{"enabled":false,"chat_id":"","token_set":false},"thresholds":{"cpu":85,"memory":90,"disk":90}}`
	rec := e.call(t, http.MethodPut, "/api/v1/settings/alerts", body, admin, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("round trip = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAlertSettingsGetIsAdminOnly(t *testing.T) {
	e := newSettingsEnv(t)
	customer := e.login(t, "customer@example.com")
	if rec := e.call(t, http.MethodGet, "/api/v1/settings/alerts", "", customer, false); rec.Code != http.StatusForbidden {
		t.Fatalf("GET as customer = %d", rec.Code)
	}
}

func TestMaintenanceToggleNeedsEnabledField(t *testing.T) {
	e := newSettingsEnv(t)
	cookie := e.login(t, testEmail)
	path := "/api/v1/sites/11111111-aaaa-4aaa-8aaa-111111111111/maintenance"
	if rec := e.call(t, http.MethodPut, path, `{}`, cookie, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled = %d, want 400", rec.Code)
	}
	if rec := e.call(t, http.MethodPut, path, `{"enabled":true}`, cookie, true); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body.String())
	}
	if e.manage.maintenance == nil || !*e.manage.maintenance {
		t.Fatal("maintenance not switched on")
	}
}

func TestAutoUpdateToggleStoresFlag(t *testing.T) {
	e := newSettingsEnv(t)
	cookie := e.login(t, testEmail)
	id := "11111111-aaaa-4aaa-8aaa-111111111111"
	rec := e.call(t, http.MethodPut, "/api/v1/sites/"+id+"/auto-update", `{"enabled":true}`, cookie, true)
	if rec.Code != http.StatusOK || !e.sites.autoUpdate[id] {
		t.Fatalf("auto-update = %d %s", rec.Code, rec.Body.String())
	}
}

func TestStartBackupReturnsAccepted(t *testing.T) {
	e := newSettingsEnv(t)
	cookie := e.login(t, testEmail)
	rec := e.call(t, http.MethodPost, "/api/v1/sites/11111111-aaaa-4aaa-8aaa-111111111111/backup", "", cookie, true)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"job_id":"job-9"`) {
		t.Fatalf("backup = %d %s", rec.Code, rec.Body.String())
	}
}

var _ AlertManager = (*fakeAlerts)(nil)
var _ SiteManager = (*settingsManage)(nil)

func TestAlertTestEmailStatusCodes(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	customer := e.login(t, "customer@example.com")
	const path = "/api/v1/settings/alerts/test-email"

	if rec := e.call(t, http.MethodPost, path, "", admin, false); rec.Code != http.StatusForbidden {
		t.Fatalf("without CSRF = %d, want 403", rec.Code)
	}
	if rec := e.call(t, http.MethodPost, path, "", customer, true); rec.Code != http.StatusForbidden {
		t.Fatalf("as customer = %d, want 403", rec.Code)
	}
	if rec := e.call(t, http.MethodPost, path, "", admin, true); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sent":true`) {
		t.Fatalf("success = %d %s", rec.Code, rec.Body.String())
	}

	e.alert.testErr = alerts.ErrEmailNotConfigured
	if rec := e.call(t, http.MethodPost, path, "", admin, true); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "email_not_configured" {
		t.Fatalf("not configured = %d %s", rec.Code, rec.Body.String())
	}

	e.alert.testErr = &alerts.DeliveryError{Message: "SMTP server refused the sender address"}
	rec := e.call(t, http.MethodPost, path, "", admin, true)
	if rec.Code != http.StatusBadGateway || errorCode(t, rec) != "email_failed" || strings.Contains(rec.Body.String(), "smtp-pass") {
		t.Fatalf("delivery failure = %d %s", rec.Code, rec.Body.String())
	}
	if e.alert.testSent != 3 {
		t.Fatalf("sends = %d, want 3", e.alert.testSent)
	}
	last := e.fs.audits[len(e.fs.audits)-1]
	if last.Action != "alerts.test_email" || last.Result != "failure" || last.ErrorCode != "email_failed" {
		t.Fatalf("audit = %+v", last)
	}
}

func (s *settingsSites) GetSFTPPublicKey(context.Context) (string, error) {
	if s.sftpKey == "" {
		return "", store.ErrNotFound
	}
	return s.sftpKey, nil
}
func (s *settingsSites) SetSFTPPublicKey(_ context.Context, _, key string) error {
	s.sftpKey = key
	return nil
}

func (m *settingsManage) SFTPPublicKey(context.Context) (string, error) {
	return "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl zenex-backup", nil
}
func (m *settingsManage) TestSFTP(_ context.Context, dest store.SFTPDestination) error {
	m.tested = append(m.tested, dest)
	return m.testErr
}
