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
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/auth"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// fakeStore is an in-memory AuthStore for handler tests.
type fakeStore struct {
	users    map[string]*store.User // by email
	sessions map[string]*store.User // by hex(token hash)
	audits   []store.AuditEntry
}

func newFakeStore(t *testing.T, email, password string) *fakeStore {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeStore{
		users: map[string]*store.User{
			email: {ID: "11111111-1111-1111-1111-111111111111", Email: email, PasswordHash: hash, Status: "active", Roles: []string{"administrator"}},
		},
		sessions: map[string]*store.User{},
	}
}

func (f *fakeStore) UserByEmail(_ context.Context, email string) (*store.User, error) {
	if u, ok := f.users[email]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) RecordFailedLogin(_ context.Context, id string, max int, lockFor time.Duration) error {
	for _, u := range f.users {
		if u.ID == id {
			u.FailedLogins++
			if u.FailedLogins >= max {
				until := time.Now().Add(lockFor)
				u.LockedUntil = &until
			}
		}
	}
	return nil
}

func (f *fakeStore) RecordSuccessfulLogin(_ context.Context, id string) error {
	for _, u := range f.users {
		if u.ID == id {
			u.FailedLogins = 0
			u.LockedUntil = nil
		}
	}
	return nil
}

func (f *fakeStore) CreateSession(_ context.Context, in store.SessionInput) error {
	for _, u := range f.users {
		if u.ID == in.UserID {
			f.sessions[string(in.TokenHash)] = u
			return nil
		}
	}
	return store.ErrNotFound
}

func (f *fakeStore) SessionUser(_ context.Context, hash []byte) (*store.User, error) {
	if u, ok := f.sessions[string(hash)]; ok {
		return u, nil
	}
	return nil, store.ErrNotFound
}

func (f *fakeStore) RevokeSession(_ context.Context, hash []byte) error {
	delete(f.sessions, string(hash))
	return nil
}

func (f *fakeStore) Audit(_ context.Context, e store.AuditEntry) error {
	f.audits = append(f.audits, e)
	return nil
}

func (f *fakeStore) Ping(_ context.Context) error { return nil }

func newTestRouter(fs AuthStore) http.Handler {
	return NewRouter(Deps{
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users: fs,
	})
}

const (
	testEmail    = "admin@example.com"
	testPassword = "correct horse battery"
)

func postLogin(h http.Handler, body string, csrf bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.7:5555"
	if csrf {
		req.Header.Set("X-Requested-With", "zenex")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookieFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	return nil
}

func TestHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Status != "ok" {
		t.Fatalf("bad health body: %s", rec.Body.String())
	}
	if rec.Header().Get("X-Request-ID") == "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing request id or security headers")
	}
}

func TestLoginSuccessSetsSecureCookieAndMe(t *testing.T) {
	fs := newFakeStore(t, testEmail, testPassword)
	h := newTestRouter(fs)

	rec := postLogin(h, `{"email":"admin@example.com","password":"correct horse battery"}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
	c := sessionCookieFrom(rec)
	if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Value == "" {
		t.Fatalf("session cookie missing or weakly configured: %+v", c)
	}
	if len(fs.sessions) != 1 {
		t.Fatal("session not stored")
	}
	for k := range fs.sessions {
		if strings.Contains(k, c.Value) {
			t.Fatal("raw token stored instead of hash")
		}
	}

	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.AddCookie(c)
	meRec := httptest.NewRecorder()
	h.ServeHTTP(meRec, me)
	if meRec.Code != http.StatusOK || !strings.Contains(meRec.Body.String(), testEmail) {
		t.Fatalf("me = %d %s", meRec.Code, meRec.Body.String())
	}
}

func TestLoginWrongPasswordIsGeneric(t *testing.T) {
	fs := newFakeStore(t, testEmail, testPassword)
	h := newTestRouter(fs)

	wrong := postLogin(h, `{"email":"admin@example.com","password":"nope nope nope"}`, true)
	unknown := postLogin(h, `{"email":"ghost@example.com","password":"nope nope nope"}`, true)
	if wrong.Code != http.StatusUnauthorized || unknown.Code != http.StatusUnauthorized {
		t.Fatalf("statuses %d / %d", wrong.Code, unknown.Code)
	}
	if wrong.Body.String() != unknown.Body.String() && !strings.Contains(wrong.Body.String(), "invalid_credentials") {
		t.Fatal("wrong-password and unknown-email responses differ")
	}
}

func TestLoginRequiresCSRFHeader(t *testing.T) {
	h := newTestRouter(newFakeStore(t, testEmail, testPassword))
	rec := postLogin(h, `{"email":"admin@example.com","password":"correct horse battery"}`, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("login without CSRF header = %d, want 403", rec.Code)
	}
}

func TestLoginRejectsUnknownFields(t *testing.T) {
	h := newTestRouter(newFakeStore(t, testEmail, testPassword))
	rec := postLogin(h, `{"email":"admin@example.com","password":"x","role":"administrator"}`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAccountLocksAfterRepeatedFailures(t *testing.T) {
	fs := newFakeStore(t, testEmail, testPassword)
	h := NewRouter(Deps{
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:        fs,
		LoginLimiter: newFixedWindowLimiter(100, time.Minute),
	})
	bad := `{"email":"admin@example.com","password":"wrong wrong wrong"}`
	for i := 0; i < maxLoginFailures; i++ {
		postLogin(h, bad, true)
	}
	rec := postLogin(h, `{"email":"admin@example.com","password":"correct horse battery"}`, true)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "account_locked") {
		t.Fatalf("locked account login = %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginRateLimitedPerIP(t *testing.T) {
	h := newTestRouter(newFakeStore(t, testEmail, testPassword))
	var last *httptest.ResponseRecorder
	for i := 0; i < 11; i++ {
		last = postLogin(h, `{"email":"x@example.com","password":"wrong wrong wrong"}`, true)
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("11th attempt = %d, want 429", last.Code)
	}
}

func TestMetricsRequiresSession(t *testing.T) {
	h := newTestRouter(newFakeStore(t, testEmail, testPassword))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/system/metrics", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("metrics without session = %d, want 401", rec.Code)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	fs := newFakeStore(t, testEmail, testPassword)
	h := newTestRouter(fs)
	c := sessionCookieFrom(postLogin(h, `{"email":"admin@example.com","password":"correct horse battery"}`, true))

	out := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	out.Header.Set("X-Requested-With", "zenex")
	out.AddCookie(c)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, out)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}

	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.AddCookie(c)
	meRec := httptest.NewRecorder()
	h.ServeHTTP(meRec, me)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session still accepted: %d", meRec.Code)
	}
}

func TestUnknownAPIPathReturnsStructuredError(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code != "not_found" {
		t.Fatalf("bad error envelope: %s", rec.Body.String())
	}
}

func TestIndexServesUIWithStrictCSP(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Zenex Panel") {
		t.Fatalf("index not served: %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("UI CSP missing script-src 'self'")
	}
}

// The page must reference only assets the server actually serves, under /assets/.
func TestPageReferencesServedAssets(t *testing.T) {
	r := newTestRouter(nil)
	index := httptest.NewRecorder()
	r.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))

	html := index.Body.String()
	found := false
	for _, marker := range []string{`src="/assets/`, `href="/assets/`} {
		rest := html
		for {
			i := strings.Index(rest, marker)
			if i < 0 {
				break
			}
			rest = rest[i+len(marker)-len("/assets/"):]
			end := strings.IndexByte(rest, '"')
			if end < 0 {
				break
			}
			path := rest[:end]
			rest = rest[end:]
			found = true
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Errorf("referenced asset %s returned %d", path, rec.Code)
			}
		}
	}
	if !found {
		t.Fatal("index.html references no /assets/ files")
	}
}
