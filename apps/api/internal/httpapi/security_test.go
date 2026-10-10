package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

const securityBase = "/api/v1/sites/33333333-3333-3333-3333-333333333333/security"

// securityManage records scanner calls.
type securityManage struct {
	settingsManage
	calls []string
	err   error
}

func (m *securityManage) StartScan(context.Context, store.Site) error {
	m.calls = append(m.calls, "scan")
	return m.err
}
func (m *securityManage) SecurityStatus(context.Context, store.Site) (manage.SecurityStatus, error) {
	m.calls = append(m.calls, "status")
	return manage.SecurityStatus{Quarantine: []manage.QuarantineItem{}}, m.err
}
func (m *securityManage) Quarantine(_ context.Context, _ store.Site, path, rule string) error {
	m.calls = append(m.calls, "quarantine:"+path+":"+rule)
	return m.err
}
func (m *securityManage) RestoreQuarantined(_ context.Context, _ store.Site, id string) error {
	m.calls = append(m.calls, "restore:"+id)
	return m.err
}
func (m *securityManage) DeleteQuarantined(_ context.Context, _ store.Site, id string) error {
	m.calls = append(m.calls, "delete:"+id)
	return m.err
}

func newSecurityEnv(t *testing.T, sm *securityManage) settingsEnv {
	t.Helper()
	ps := &progressSites{}
	e := newProgressEnv(t, ps)
	e.sites.siteState = "ready"
	e.h = NewRouter(Deps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:  e.fs,
		Sites:  ps,
		Manage: sm,
		Alerts: e.alert,
		Site:   SiteSettings{NodeID: "node-1", PHPVersion: "8.3", SecretKey: []byte(testServerSecret), StartJob: func(string) {}},
	})
	return e
}

func TestSecurityEndpointsRequireSignIn(t *testing.T) {
	sm := &securityManage{}
	e := newSecurityEnv(t, sm)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, securityBase},
		{http.MethodPost, securityBase + "/scan"},
		{http.MethodPost, securityBase + "/quarantine"},
	} {
		if rec := e.call(t, c.method, c.path, "{}", nil, true); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", c.method, c.path, rec.Code)
		}
	}
	if len(sm.calls) != 0 {
		t.Fatalf("scanner ran without a session: %v", sm.calls)
	}
}

func TestSecurityMutationsNeedTheCSRFHeader(t *testing.T) {
	sm := &securityManage{}
	e := newSecurityEnv(t, sm)
	admin := e.login(t, testEmail)
	rec := e.call(t, http.MethodPost, securityBase+"/scan", "", admin, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(sm.calls) != 0 {
		t.Fatalf("scan started without the CSRF header: %v", sm.calls)
	}
}

func TestSecurityFlow(t *testing.T) {
	sm := &securityManage{}
	e := newSecurityEnv(t, sm)
	admin := e.login(t, testEmail)
	id := strings.Repeat("c", 24)

	if rec := e.call(t, http.MethodGet, securityBase, "", admin, false); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.call(t, http.MethodPost, securityBase+"/scan", "", admin, true); rec.Code != http.StatusAccepted {
		t.Fatalf("scan = %d: %s", rec.Code, rec.Body.String())
	}
	body := `{"path":"wp-content/uploads/x.php","rule":"request-exec"}`
	if rec := e.call(t, http.MethodPost, securityBase+"/quarantine", body, admin, true); rec.Code != http.StatusNoContent {
		t.Fatalf("quarantine = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := e.call(t, http.MethodPost, securityBase+"/quarantine/"+id+"/restore", "", admin, true); rec.Code != http.StatusNoContent {
		t.Fatalf("restore = %d", rec.Code)
	}
	if rec := e.call(t, http.MethodDelete, securityBase+"/quarantine/"+id, "", admin, true); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	want := []string{"status", "scan", "quarantine:wp-content/uploads/x.php:request-exec", "restore:" + id, "delete:" + id}
	if strings.Join(sm.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %v, want %v", sm.calls, want)
	}
}

func TestSecurityRejectsBadInput(t *testing.T) {
	sm := &securityManage{}
	e := newSecurityEnv(t, sm)
	admin := e.login(t, testEmail)

	bad := []string{`{}`, `{"path":""}`, `{"path":"a.php","rule":"../x"}`, `not json`, `{"path":"` + strings.Repeat("a", 2000) + `"}`}
	for _, b := range bad {
		if rec := e.call(t, http.MethodPost, securityBase+"/quarantine", b, admin, true); rec.Code != http.StatusBadRequest {
			t.Errorf("body %.30q = %d, want 400", b, rec.Code)
		}
	}
	for _, item := range []string{"short", strings.Repeat("G", 24), strings.Repeat("a", 25)} {
		if rec := e.call(t, http.MethodPost, securityBase+"/quarantine/"+item+"/restore", "", admin, true); rec.Code != http.StatusNotFound {
			t.Errorf("restore %q = %d, want 404", item, rec.Code)
		}
		if rec := e.call(t, http.MethodDelete, securityBase+"/quarantine/"+item, "", admin, true); rec.Code != http.StatusNotFound {
			t.Errorf("delete %q = %d, want 404", item, rec.Code)
		}
	}
	if len(sm.calls) != 0 {
		t.Fatalf("scanner ran for bad input: %v", sm.calls)
	}
}

func TestSecurityRefusalIsShownAsConflict(t *testing.T) {
	sm := &securityManage{err: &manage.Refusal{Message: "a scan of this website is already running"}}
	e := newSecurityEnv(t, sm)
	admin := e.login(t, testEmail)
	rec := e.call(t, http.MethodPost, securityBase+"/scan", "", admin, true)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "already running") {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}
