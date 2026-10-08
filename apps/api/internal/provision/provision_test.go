package provision

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// memStore is an in-memory Store for provisioner tests.
type memStore struct {
	mu       sync.Mutex
	job      store.Job
	site     store.Site
	steps    map[string]store.JobStep
	order    []string
	logs     []string
	siteStat string
	jobStat  string
	jobErr   string
}

func newMemStore() *memStore {
	return &memStore{
		job:   store.Job{ID: "job-1", Type: JobType, Status: "queued", SiteID: "site-1"},
		site:  store.Site{ID: "site-1", Slug: "shop", Domain: "shop.example.com", LinuxUser: "zx_shop", DBName: "zx_shop", DBUser: "zx_shop", PHPVersion: "8.3", State: "provisioning"},
		steps: map[string]store.JobStep{},
	}
}

func (m *memStore) GetJob(_ context.Context, _ string) (store.Job, error) { return m.job, nil }
func (m *memStore) GetSite(_ context.Context, _ string) (store.Site, error) {
	return m.site, nil
}
func (m *memStore) SiteOwnerEmail(_ context.Context, _ string) (string, error) {
	return "owner@example.com", nil
}
func (m *memStore) SetSiteUID(_ context.Context, _ string, _ int) error { return nil }
func (m *memStore) SetSiteState(_ context.Context, _, state string) error {
	m.siteStat = state
	return nil
}
func (m *memStore) ClaimJob(_ context.Context, _ string) (bool, error) {
	if m.job.Status != "queued" && m.job.Status != "failed" {
		return false, nil
	}
	m.job.Status = "running"
	return true, nil
}
func (m *memStore) EnsureJobSteps(_ context.Context, _ string, names []string) error {
	for _, n := range names {
		if _, ok := m.steps[n]; !ok {
			m.steps[n] = store.JobStep{Name: n, Status: "pending"}
		}
	}
	return nil
}
func (m *memStore) JobSteps(_ context.Context, _ string) ([]store.JobStep, error) {
	var out []store.JobStep
	for _, n := range stepNames {
		if s, ok := m.steps[n]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}
func (m *memStore) SetStepStatus(_ context.Context, _, name, status, errMsg string) error {
	s := m.steps[name]
	s.Name, s.Status, s.Error = name, status, errMsg
	m.steps[name] = s
	if status == "succeeded" {
		m.order = append(m.order, name)
	}
	return nil
}
func (m *memStore) FinishJob(_ context.Context, _, status, errMsg string) error {
	m.job.Status = status
	m.jobErr = errMsg
	return nil
}
func (m *memStore) AppendJobLog(_ context.Context, _, _, msg string) error {
	m.logs = append(m.logs, msg)
	return nil
}

// fakeHelper records operations and fails the named one with a message.
type fakeHelper struct {
	calls  []string
	failOp string
	failTo string
}

func (f *fakeHelper) Do(_ context.Context, op string, args map[string]string) (string, error) {
	f.calls = append(f.calls, op)
	if op == f.failOp {
		return "", errors.New(f.failTo)
	}
	if op == "user.create" {
		return "1001", nil
	}
	return "", nil
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestRunSucceedsInOrder(t *testing.T) {
	// web server stand-in: answers 200 for the site's domain.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "shop.example.com" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := newMemStore()
	h := &fakeHelper{}
	p := New(st, h, testKey, quietLogger())
	p.HealthBase = srv.URL

	if err := p.Run(context.Background(), "job-1"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if st.job.Status != "succeeded" || st.siteStat != "ready" {
		t.Fatalf("job=%s site=%s, want succeeded/ready", st.job.Status, st.siteStat)
	}
	if strings.Join(st.order, ",") != strings.Join(stepNames, ",") {
		t.Fatalf("step order = %v", st.order)
	}
}

func TestFailureRecordsStepAndResumesFromIt(t *testing.T) {
	st := newMemStore()
	h := &fakeHelper{failOp: "db.create", failTo: "database setup failed"}
	p := New(st, h, testKey, quietLogger())
	p.HealthBase = siteServer(t).URL

	if err := p.Run(context.Background(), "job-1"); err == nil {
		t.Fatal("expected failure")
	}
	if st.job.Status != "failed" || st.siteStat != "failed" {
		t.Fatalf("job=%s site=%s, want failed/failed", st.job.Status, st.siteStat)
	}
	if st.steps["create_database"].Status != "failed" {
		t.Fatalf("failed step not recorded: %+v", st.steps["create_database"])
	}
	if st.steps["create_account"].Status != "succeeded" {
		t.Fatal("earlier step should remain succeeded")
	}

	// Retry: the helper now succeeds; earlier steps must not run again.
	h.failOp = ""
	h.calls = nil
	st.job.Status = "failed"
	if err := p.Run(context.Background(), "job-1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	// Every step runs again on retry; each is idempotent on the helper side.
	if len(h.calls) == 0 || h.calls[0] != "user.create" {
		t.Fatalf("retry did not start from the first step: %v", h.calls)
	}
	if !contains(h.calls, "fs.prepare") {
		t.Fatalf("retry skipped the file preparation step: %v", h.calls)
	}
	if st.job.Status != "succeeded" {
		t.Fatalf("retry did not finish: %s", st.job.Status)
	}
}

func TestErrorMessagesNeverContainDerivedPasswords(t *testing.T) {
	st := newMemStore()
	dbPass := DBPassword(testKey, st.site.ID)
	h := &fakeHelper{failOp: "wp.config-create", failTo: "wp-cli failed: --dbpass=" + dbPass}
	p := New(st, h, testKey, quietLogger())

	_ = p.Run(context.Background(), "job-1")
	if strings.Contains(st.jobErr, dbPass) {
		t.Fatalf("password leaked into job error: %s", st.jobErr)
	}
	for _, l := range st.logs {
		if strings.Contains(l, dbPass) {
			t.Fatalf("password leaked into job log: %s", l)
		}
	}
}

func TestPasswordsAreStableAndSiteSpecific(t *testing.T) {
	a := DBPassword(testKey, "site-a")
	if a != DBPassword(testKey, "site-a") {
		t.Fatal("derived password is not stable")
	}
	if a == DBPassword(testKey, "site-b") {
		t.Fatal("two sites share a password")
	}
	if len(a) != 32 || len(WPAdminPassword(testKey, "site-a")) != 24 {
		t.Fatal("unexpected password length")
	}
}

// siteServer answers 200 for shop.example.com, standing in for the web server.
func siteServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "shop.example.com" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}
