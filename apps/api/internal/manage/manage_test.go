package manage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

type fakeStore struct{ state string }

func (f *fakeStore) SetSiteState(_ context.Context, _, s string) error { f.state = s; return nil }
func (f *fakeStore) SetSitePHPVersion(context.Context, string, string) error {
	return nil
}
func (f *fakeStore) MarkSiteDeleted(context.Context, string) error { f.state = "deleted"; return nil }
func (f *fakeStore) CreateManagementJob(context.Context, string, string, string, string) (string, error) {
	return "job-1", nil
}
func (f *fakeStore) FinishJob(context.Context, string, string, string) error    { return nil }
func (f *fakeStore) AppendJobLog(context.Context, string, string, string) error { return nil }

type fakeHelper struct{ calls []string }

func (f *fakeHelper) Do(_ context.Context, op string, _ map[string]string) (string, error) {
	f.calls = append(f.calls, op)
	return "", nil
}
func (f *fakeHelper) Output(_ context.Context, op string, _ map[string]string) (string, error) {
	f.calls = append(f.calls, op)
	return "", nil
}

func newTest(state string) (*Manager, *fakeStore, *fakeHelper, store.Site) {
	st := &fakeStore{state: state}
	h := &fakeHelper{}
	m := New(st, h, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return m, st, h, store.Site{ID: "s1", LinuxUser: "zx_shop", State: state, PHPVersion: "8.3"}
}

func asRefusal(err error) bool {
	var r *Refusal
	return errors.As(err, &r)
}

func TestCannotSuspendWhileBuilding(t *testing.T) {
	m, _, h, site := newTest("provisioning")
	if err := m.Suspend(context.Background(), site); !asRefusal(err) {
		t.Fatalf("expected refusal, got %v", err)
	}
	if len(h.calls) != 0 {
		t.Fatal("helper ran for a refused action")
	}
}

func TestSuspendThenResume(t *testing.T) {
	m, st, h, site := newTest("ready")
	if err := m.Suspend(context.Background(), site); err != nil {
		t.Fatal(err)
	}
	if st.state != "suspended" || h.calls[0] != "vhost.disable" {
		t.Fatalf("state=%s calls=%v", st.state, h.calls)
	}
	site.State = "suspended"
	if err := m.Resume(context.Background(), site); err != nil {
		t.Fatal(err)
	}
	if st.state != "ready" {
		t.Fatalf("state after resume = %s", st.state)
	}
}

func TestCannotResumeReadySite(t *testing.T) {
	m, _, _, site := newTest("ready")
	if err := m.Resume(context.Background(), site); !asRefusal(err) {
		t.Fatalf("expected refusal, got %v", err)
	}
}

func TestCannotDeleteWhileBuilding(t *testing.T) {
	m, _, _, site := newTest("provisioning")
	if _, err := m.Delete(context.Background(), site, "u1"); !asRefusal(err) {
		t.Fatalf("expected refusal, got %v", err)
	}
}

func TestSwitchPHPRejectsSameVersion(t *testing.T) {
	m, _, _, site := newTest("ready")
	if err := m.SwitchPHP(context.Background(), site, "8.3"); !asRefusal(err) {
		t.Fatalf("expected refusal, got %v", err)
	}
}

func (f *fakeStore) Notify(context.Context, string, string, string, string) error { return nil }
func (f *fakeStore) SetSiteMaintenance(context.Context, string, bool) error       { return nil }
func (f *fakeStore) ListReadySites(context.Context) ([]store.Site, error)         { return nil, nil }
func (f *fakeStore) ListAutoUpdateSites(context.Context) ([]store.Site, error)    { return nil, nil }
func (f *fakeStore) InsertBackup(context.Context, string, int64, string) error    { return nil }
func (f *fakeStore) BackupsBefore(context.Context, time.Time) ([]store.Backup, error) {
	return nil, nil
}
func (f *fakeStore) DeleteBackup(context.Context, int64) error { return nil }
func (f *fakeStore) GetBackupSettings(context.Context) (store.BackupSettings, error) {
	return store.DefaultBackupSettings(), nil
}
