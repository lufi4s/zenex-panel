package manage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// featureStore records maintenance, backup and auto-update calls on top of the shared fake.
type featureStore struct {
	*fakeStore
	maintenance map[string]bool
	backups     []store.Backup
	deleted     []int64
	autoUpdate  []store.Site
	inserted    []string
}

func (f *featureStore) SetSiteMaintenance(_ context.Context, id string, on bool) error {
	f.maintenance[id] = on
	return nil
}
func (f *featureStore) ListAutoUpdateSites(context.Context) ([]store.Site, error) {
	return f.autoUpdate, nil
}
func (f *featureStore) InsertBackup(_ context.Context, siteID string, _ int64, path string) error {
	f.inserted = append(f.inserted, siteID+"|"+path)
	return nil
}
func (f *featureStore) BackupsBefore(_ context.Context, cutoff time.Time) ([]store.Backup, error) {
	var out []store.Backup
	for _, b := range f.backups {
		if b.CreatedAt.Before(cutoff) {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *featureStore) DeleteBackup(_ context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

// scriptedHelper records every call with its arguments and can be told to fail an op.
type scriptedHelper struct {
	fakeHelper
	outputs map[string]string
	fail    map[string]error
	args    map[string][]map[string]string
}

func (h *scriptedHelper) Do(_ context.Context, op string, args map[string]string) (string, error) {
	h.record(op, args)
	return "", h.fail[op]
}

func (h *scriptedHelper) Output(_ context.Context, op string, args map[string]string) (string, error) {
	h.record(op, args)
	if err := h.fail[op]; err != nil {
		return "", err
	}
	return h.outputs[op], nil
}

func (h *scriptedHelper) record(op string, args map[string]string) {
	h.calls = append(h.calls, op)
	if h.args == nil {
		h.args = map[string][]map[string]string{}
	}
	h.args[op] = append(h.args[op], args)
}

func newFeatureTest(state string) (*Manager, *featureStore, *scriptedHelper) {
	fs := &featureStore{fakeStore: &fakeStore{state: state}, maintenance: map[string]bool{}}
	h := &scriptedHelper{outputs: map[string]string{}, fail: map[string]error{}}
	m := New(fs, h, quietLogger())
	return m, fs, h
}

func TestSetMaintenancePassesFlagToVhostWrite(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", Domain: "shop.example.com", State: "ready"}
	if err := m.SetMaintenance(context.Background(), site, true); err != nil {
		t.Fatal(err)
	}
	args := h.args["vhost.write"][0]
	if args["user"] != "zx_shop" || args["domain"] != "shop.example.com" || args["maintenance"] != "1" {
		t.Fatalf("vhost.write args = %v", args)
	}
	if !st.maintenance["s1"] {
		t.Fatal("flag not stored after the web server accepted the change")
	}
}

func TestSetMaintenanceKeepsFlagWhenHelperFails(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	h.fail["vhost.write"] = errors.New("config rejected")
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", Domain: "shop.example.com", State: "ready"}
	if err := m.SetMaintenance(context.Background(), site, true); err == nil {
		t.Fatal("helper failure was not returned")
	}
	if len(st.maintenance) != 0 {
		t.Fatal("flag stored although the change failed")
	}
}

func TestSetMaintenanceRefusesSuspendedSite(t *testing.T) {
	m, _, h := newFeatureTest("suspended")
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", Domain: "shop.example.com", State: "suspended"}
	if err := m.SetMaintenance(context.Background(), site, true); !asRefusal(err) {
		t.Fatalf("expected refusal, got %v", err)
	}
	if len(h.calls) != 0 {
		t.Fatal("suspended site's configuration was rewritten")
	}
}

func TestParseBackupOutput(t *testing.T) {
	path, size, err := parseBackupOutput(BackupRoot + "/zx_shop/20261009-030000.tar.gz 1048576")
	if err != nil || path != BackupRoot+"/zx_shop/20261009-030000.tar.gz" || size != 1048576 {
		t.Fatalf("parse = %q %d %v", path, size, err)
	}
	for _, bad := range []string{
		"",
		"only-one-field",
		"/etc/passwd 10",
		BackupRoot + "/../../etc/passwd 10",
		BackupRoot + "/zx_shop/a.tar.gz -5",
		BackupRoot + "/zx_shop/a.tar.gz notanumber",
	} {
		if _, _, err := parseBackupOutput(bad); err == nil {
			t.Errorf("accepted bad reply %q", bad)
		}
	}
}

func TestStartBackupRequiresReadySite(t *testing.T) {
	m, _, _ := newFeatureTest("provisioning")
	site := store.Site{ID: "s1", State: "provisioning"}
	if _, err := m.StartBackup(context.Background(), site, "u1"); !asRefusal(err) {
		t.Fatalf("expected refusal, got %v", err)
	}
}

func TestBackupRunsHelperAndRecordsArchive(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	h.outputs["backup.create"] = BackupRoot + "/zx_shop/x.tar.gz 2048"
	site := store.Site{ID: "s1", NodeID: "n1", LinuxUser: "zx_shop", DBName: "zx_shop", OwnerID: "u1", State: "ready", Domain: "shop.example.com"}

	m.runBackup(context.Background(), site, "job-1", "u1")

	args := h.args["backup.create"][0]
	if args["user"] != "zx_shop" || args["database"] != "zx_shop" || args["docroot"] != "/var/www/zx_shop/htdocs" {
		t.Fatalf("backup.create args = %v", args)
	}
	if !strings.HasPrefix(args["output"], BackupRoot+"/zx_shop/") || !strings.HasSuffix(args["output"], ".tar.gz") {
		t.Fatalf("output path = %q", args["output"])
	}
	if len(st.inserted) != 1 || st.inserted[0] != "s1|"+BackupRoot+"/zx_shop/x.tar.gz" {
		t.Fatalf("archive not recorded: %v", st.inserted)
	}
}

func TestScheduledBackupsPruneExpiredArchives(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	st.backups = []store.Backup{
		{ID: 1, CreatedAt: now.AddDate(0, 0, -10), Path: BackupRoot + "/zx_a/old.tar.gz"},
		{ID: 2, CreatedAt: now.AddDate(0, 0, -1), Path: BackupRoot + "/zx_a/recent.tar.gz"},
	}
	m.PruneBackups(context.Background(), now, 7)
	if len(h.args["backup.delete"]) != 1 || h.args["backup.delete"][0]["path"] != BackupRoot+"/zx_a/old.tar.gz" {
		t.Fatalf("backup.delete calls = %v", h.args["backup.delete"])
	}
	if len(st.deleted) != 1 || st.deleted[0] != 1 {
		t.Fatalf("deleted records = %v", st.deleted)
	}
}

func TestFailedArchiveDeletionKeepsRecord(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	st.backups = []store.Backup{{ID: 7, CreatedAt: now.AddDate(0, 0, -30), Path: BackupRoot + "/zx_a/x.tar.gz"}}
	h.fail["backup.delete"] = errors.New("busy")
	m.PruneBackups(context.Background(), now, 7)
	if len(st.deleted) != 0 {
		t.Fatal("record removed although the archive still exists")
	}
}

func TestAutoUpdatesRunWordPressUpdateForEachSite(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	st.autoUpdate = []store.Site{
		{ID: "s1", LinuxUser: "zx_a", OwnerID: "u1", Domain: "a.example.com"},
		{ID: "s2", LinuxUser: "zx_b", OwnerID: "u2", Domain: "b.example.com"},
	}
	h.fail["wp.update"] = errors.New("update failed")
	m.RunAutoUpdates(context.Background())
	if len(h.args["wp.update"]) != 2 {
		t.Fatalf("wp.update calls = %d, want 2 (one failure must not stop the rest)", len(h.args["wp.update"]))
	}
	if h.args["wp.update"][1]["path"] != "/var/www/zx_b/htdocs" {
		t.Fatalf("path = %q", h.args["wp.update"][1]["path"])
	}
}

func TestAutoUpdatesStopWhenCancelled(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	st.autoUpdate = []store.Site{{ID: "s1", LinuxUser: "zx_a"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.RunAutoUpdates(ctx)
	if len(h.calls) != 0 {
		t.Fatal("updates ran after cancellation")
	}
}
