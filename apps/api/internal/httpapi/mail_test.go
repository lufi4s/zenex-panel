package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

const (
	mailZoneID    = "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa"
	mailOtherZone = "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb"
	mailAdminID   = "11111111-1111-1111-1111-111111111111"
	mailCustID    = "55555555-5555-5555-5555-555555555555"
	goodPassword  = "correct horse battery"
)

// fakeMail is an in-memory MailStore for the handler tests.
type fakeMail struct {
	mu      sync.Mutex
	zones   map[string]store.ZoneInfo
	enabled map[string]bool
	boxes   map[string]store.Mailbox
	dkim    map[string]string
	host    string
	seq     int
}

func newFakeMail() *fakeMail {
	return &fakeMail{
		zones: map[string]store.ZoneInfo{
			mailZoneID:    {ID: mailZoneID, OwnerID: mailCustID, Apex: "ozima.cloud"},
			mailOtherZone: {ID: mailOtherZone, OwnerID: mailAdminID, Apex: "other.example"},
		},
		enabled: map[string]bool{},
		boxes:   map[string]store.Mailbox{},
		dkim:    map[string]string{},
	}
}

func (f *fakeMail) GetMailHost(context.Context) (string, error) { return f.host, nil }

func (f *fakeMail) SetMailHost(_ context.Context, host, _ string) error {
	f.host = host
	return nil
}

func (f *fakeMail) SetMailDKIM(_ context.Context, id, value string) error {
	f.dkim[id] = value
	return nil
}

func (f *fakeMail) ZoneOwner(_ context.Context, id string) (store.ZoneInfo, error) {
	z, ok := f.zones[id]
	if !ok {
		return store.ZoneInfo{}, store.ErrNotFound
	}
	return z, nil
}

func (f *fakeMail) GetMailDomain(_ context.Context, id string) (store.MailDomain, error) {
	return store.MailDomain{ZoneID: id, Enabled: f.enabled[id], DKIMPublic: f.dkim[id]}, nil
}

// emptyDNS answers every lookup with "not found", so tests never ask the real DNS.
type emptyDNS struct{}

func (emptyDNS) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	return nil, &net.DNSError{Name: name, IsNotFound: true}
}

func (emptyDNS) LookupTXT(_ context.Context, name string) ([]string, error) {
	return nil, &net.DNSError{Name: name, IsNotFound: true}
}

func (emptyDNS) LookupHost(_ context.Context, name string) ([]string, error) {
	return nil, &net.DNSError{Name: name, IsNotFound: true}
}

// fakeMailServer records the hosts it was set up for and answers DKIM requests.
type fakeMailServer struct {
	hosts    []string
	applyErr error
	dkimErr  error
}

func (f *fakeMailServer) ApplyMailServer(_ context.Context, host string) error {
	f.hosts = append(f.hosts, host)
	return f.applyErr
}

func (f *fakeMailServer) MailDKIMValue(context.Context, string) (string, error) {
	if f.dkimErr != nil {
		return "", f.dkimErr
	}
	return "p=QUJD", nil
}

func (f *fakeMail) SetMailDomain(_ context.Context, id string, on bool) error {
	f.enabled[id] = on
	return nil
}

func (f *fakeMail) ListMailboxes(_ context.Context, zone string) ([]store.Mailbox, error) {
	out := []store.Mailbox{}
	for _, b := range f.boxes {
		if b.ZoneID == zone {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeMail) CreateMailbox(_ context.Context, mb store.Mailbox) (store.Mailbox, error) {
	for _, b := range f.boxes {
		if b.ZoneID == mb.ZoneID && b.LocalPart == mb.LocalPart {
			return store.Mailbox{}, store.ErrConflict
		}
	}
	f.seq++
	mb.ID = fmt.Sprintf("cccccccc-0000-4000-8000-%012d", f.seq)
	mb.CreatedAt, mb.UpdatedAt = time.Now(), time.Now()
	f.boxes[mb.ID] = mb
	return mb, nil
}

func (f *fakeMail) GetMailbox(_ context.Context, id string) (store.Mailbox, error) {
	b, ok := f.boxes[id]
	if !ok {
		return store.Mailbox{}, store.ErrNotFound
	}
	return b, nil
}

func (f *fakeMail) UpdateMailbox(_ context.Context, mb store.Mailbox) error {
	f.boxes[mb.ID] = mb
	return nil
}

func (f *fakeMail) DeleteMailbox(_ context.Context, id string) error {
	delete(f.boxes, id)
	return nil
}

// newMailEnv builds the settings fixture with a Mail store attached.
func newMailEnv(t *testing.T) (settingsEnv, *fakeMail) {
	t.Helper()
	e, fm, _ := newMailEnvWithServer(t)
	return e, fm
}

func newMailEnvWithServer(t *testing.T) (settingsEnv, *fakeMail, *fakeMailServer) {
	t.Helper()
	e := newSettingsEnv(t)
	fm := newFakeMail()
	srv := &fakeMailServer{}
	e.h = NewRouter(Deps{
		MailServer:   srv,
		MailResolver: emptyDNS{},
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:        e.fs,
		Sites:        e.sites,
		Manage:       e.manage,
		Alerts:       e.alert,
		Mail:         fm,
		Site:         SiteSettings{NodeID: "node-1", PHPVersion: "8.3", SecretKey: []byte(testServerSecret), StartJob: func(string) {}, PublicIP: "162.4.35.76"},
	})
	return e, fm, srv
}

func TestMailboxNeedsMailTurnedOnFirst(t *testing.T) {
	e, _ := newMailEnv(t)
	customer := e.login(t, "customer@example.com")
	body := `{"local_part":"info","password":"` + goodPassword + `"}`
	rec := e.call(t, http.MethodPost, "/api/v1/domains/"+mailZoneID+"/mailboxes", body, customer, true)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "mail_disabled") {
		t.Fatalf("mailbox while mail is off = %d %s", rec.Code, rec.Body)
	}
}

func TestMailboxLifecycleAndPasswordStaysHidden(t *testing.T) {
	e, fm := newMailEnv(t)
	customer := e.login(t, "customer@example.com")
	zone := "/api/v1/domains/" + mailZoneID

	if rec := e.call(t, http.MethodPut, zone+"/mail", `{"enabled":true}`, customer, true); rec.Code != http.StatusOK {
		t.Fatalf("enable mail = %d %s", rec.Code, rec.Body)
	}
	rec := e.call(t, http.MethodPost, zone+"/mailboxes", `{"local_part":"info","password":"`+goodPassword+`","quota_mb":2048}`, customer, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatalf("response leaks the password field: %s", rec.Body)
	}
	var created store.Mailbox
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(fm.boxes[created.ID].PasswordHash, "$argon2id$") {
		t.Fatalf("password not stored as an Argon2id hash: %q", fm.boxes[created.ID].PasswordHash)
	}
	if created.QuotaMB != 2048 || !created.Active {
		t.Fatalf("created = %+v", created)
	}

	if rec := e.call(t, http.MethodPost, zone+"/mailboxes", `{"local_part":"info","password":"`+goodPassword+`"}`, customer, true); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "address_taken") {
		t.Fatalf("duplicate = %d %s", rec.Code, rec.Body)
	}
	if rec := e.call(t, http.MethodPost, zone+"/mailboxes", `{"local_part":"sales","password":"short"}`, customer, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("weak password = %d", rec.Code)
	}

	get := e.call(t, http.MethodGet, zone+"/mail", "", customer, false)
	if get.Code != http.StatusOK || strings.Contains(get.Body.String(), "argon2") {
		t.Fatalf("mail view = %d %s", get.Code, get.Body)
	}
	if !strings.Contains(get.Body.String(), "162.4.35.76") || !strings.Contains(get.Body.String(), "mail.ozima.cloud") {
		t.Fatalf("DNS records missing from the view: %s", get.Body)
	}

	upd := e.call(t, http.MethodPut, "/api/v1/mailboxes/"+created.ID, `{"quota_mb":4096,"active":false}`, customer, true)
	if upd.Code != http.StatusOK || !strings.Contains(upd.Body.String(), `"quota_mb":4096`) {
		t.Fatalf("update = %d %s", upd.Code, upd.Body)
	}
	if del := e.call(t, http.MethodDelete, "/api/v1/mailboxes/"+created.ID, "", customer, true); del.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", del.Code)
	}
}

func TestMailOnlyForOwnerAndNeedsCSRF(t *testing.T) {
	e, _ := newMailEnv(t)
	customer := e.login(t, "customer@example.com")
	// The admin's domain is not the customer's: the customer gets 404, not the data.
	if rec := e.call(t, http.MethodGet, "/api/v1/domains/"+mailOtherZone+"/mail", "", customer, false); rec.Code != http.StatusNotFound {
		t.Fatalf("another owner's domain = %d, want 404", rec.Code)
	}
	if rec := e.call(t, http.MethodPut, "/api/v1/domains/"+mailZoneID+"/mail", `{"enabled":true}`, customer, false); rec.Code != http.StatusForbidden {
		t.Fatalf("enable without CSRF = %d, want 403", rec.Code)
	}
	if rec := e.call(t, http.MethodGet, "/api/v1/domains/"+mailZoneID+"/mail", "", nil, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want 401", rec.Code)
	}
}

func TestEnablingMailCreatesTheDKIMKeyAndShowsItInDNS(t *testing.T) {
	e, fm, _ := newMailEnvWithServer(t)
	customer := e.login(t, "customer@example.com")
	rec := e.call(t, http.MethodPut, "/api/v1/domains/"+mailZoneID+"/mail", `{"enabled":true}`, customer, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body)
	}
	if fm.dkim[mailZoneID] != "p=QUJD" {
		t.Fatalf("DKIM public value not stored: %q", fm.dkim[mailZoneID])
	}
	if !strings.Contains(rec.Body.String(), "v=DKIM1; k=rsa; p=QUJD") {
		t.Fatalf("DKIM record not shown: %s", rec.Body)
	}
}

func TestDKIMFailureKeepsMailOff(t *testing.T) {
	e, fm, srv := newMailEnvWithServer(t)
	srv.dkimErr = errors.New("helper not running")
	customer := e.login(t, "customer@example.com")
	rec := e.call(t, http.MethodPut, "/api/v1/domains/"+mailZoneID+"/mail", `{"enabled":true}`, customer, true)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "dkim_failed") {
		t.Fatalf("DKIM failure = %d %s", rec.Code, rec.Body)
	}
	if fm.enabled[mailZoneID] {
		t.Fatal("mail was turned on although its key could not be created")
	}
}

func TestMailDNSCheckReportsWhatIsMissing(t *testing.T) {
	e, _ := newMailEnv(t)
	customer := e.login(t, "customer@example.com")
	rec := e.call(t, http.MethodGet, "/api/v1/domains/"+mailZoneID+"/mail/check", "", customer, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("check = %d %s", rec.Code, rec.Body)
	}
	var got struct {
		AllOK   bool `json:"all_ok"`
		Results []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.AllOK || len(got.Results) != 5 {
		t.Fatalf("all_ok = %v, results = %+v", got.AllOK, got.Results)
	}
	for _, r := range got.Results {
		if r.Status == "ok" {
			t.Errorf("%s reported ok with nothing published", r.Type)
		}
	}
	if rec := e.call(t, http.MethodGet, "/api/v1/domains/"+mailOtherZone+"/mail/check", "", customer, false); rec.Code != http.StatusNotFound {
		t.Fatalf("another owner's check = %d, want 404", rec.Code)
	}
}

func TestSettingUpMailAppliesTheServerThenSavesTheHost(t *testing.T) {
	e, fm, srv := newMailEnvWithServer(t)
	admin := e.login(t, "admin@example.com")
	rec := e.call(t, http.MethodPut, "/api/v1/mail/server", `{"host":"mail.ozima.cloud"}`, admin, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("set up = %d %s", rec.Code, rec.Body)
	}
	if len(srv.hosts) != 1 || srv.hosts[0] != "mail.ozima.cloud" || fm.host != "mail.ozima.cloud" {
		t.Fatalf("applied for %v, saved %q", srv.hosts, fm.host)
	}
	get := e.call(t, http.MethodGet, "/api/v1/mail/server", "", admin, false)
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), "mail.ozima.cloud") {
		t.Fatalf("get = %d %s", get.Code, get.Body)
	}
}

func TestMailHostIsNotSavedWhenTheSetupFails(t *testing.T) {
	e, fm, srv := newMailEnvWithServer(t)
	srv.applyErr = &manage.Refusal{Message: "the Postfix check failed"}
	admin := e.login(t, "admin@example.com")
	rec := e.call(t, http.MethodPut, "/api/v1/mail/server", `{"host":"mail.ozima.cloud"}`, admin, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("failed setup = %d %s", rec.Code, rec.Body)
	}
	if fm.host != "" {
		t.Fatalf("host saved after a failed setup: %q", fm.host)
	}
}

func TestMailServerSetupIsAdminOnlyAndChecksTheHost(t *testing.T) {
	e, _, srv := newMailEnvWithServer(t)
	customer := e.login(t, "customer@example.com")
	if rec := e.call(t, http.MethodPut, "/api/v1/mail/server", `{"host":"mail.ozima.cloud"}`, customer, true); rec.Code != http.StatusForbidden {
		t.Errorf("customer set up = %d, want 403", rec.Code)
	}
	if rec := e.call(t, http.MethodGet, "/api/v1/mail/server", "", customer, false); rec.Code != http.StatusForbidden {
		t.Errorf("customer read = %d, want 403", rec.Code)
	}
	admin := e.login(t, "admin@example.com")
	if rec := e.call(t, http.MethodPut, "/api/v1/mail/server", `{"host":"mail.ozima.cloud"}`, admin, false); rec.Code != http.StatusForbidden {
		t.Errorf("set up without CSRF = %d, want 403", rec.Code)
	}
	for _, bad := range []string{"localhost", "Mail.Example.com", "mail.example.com; reboot", ""} {
		body := `{"host":"` + bad + `"}`
		if rec := e.call(t, http.MethodPut, "/api/v1/mail/server", body, admin, true); rec.Code != http.StatusBadRequest {
			t.Errorf("host %q = %d, want 400", bad, rec.Code)
		}
	}
	if len(srv.hosts) != 0 {
		t.Fatalf("server set up for refused requests: %v", srv.hosts)
	}
}
