package manage

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/helperclient"
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
	settings    store.BackupSettings
	insertErr   error
}

func (f *featureStore) GetBackupSettings(context.Context) (store.BackupSettings, error) {
	return f.settings, nil
}

func (f *featureStore) SetSiteMaintenance(_ context.Context, id string, on bool) error {
	f.maintenance[id] = on
	return nil
}
func (f *featureStore) ListAutoUpdateSites(context.Context) ([]store.Site, error) {
	return f.autoUpdate, nil
}
func (f *featureStore) InsertBackup(_ context.Context, siteID string, _ int64, path string) error {
	if f.insertErr != nil {
		return f.insertErr
	}
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

var testSFTP = store.SFTPDestination{Host: "backup.example.com", Port: 2222, Username: "zx_backup", Path: "/backups/zenex"}

func sftpSettings() store.BackupSettings {
	b := store.DefaultBackupSettings()
	b.Destination = store.BackupDestination{Type: store.BackupDestSFTP, SFTP: testSFTP}
	return b
}

func TestBackupToSFTPUploadsThenRecordsRemotePath(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	st.settings = sftpSettings()
	local := BackupRoot + "/zx_shop/20261009-030000.tar.gz"
	h.outputs["backup.create"] = local + " 4096"
	site := store.Site{ID: "s1", NodeID: "n1", LinuxUser: "zx_shop", DBName: "zx_shop", OwnerID: "u1", State: "ready", Domain: "shop.example.com"}

	m.runBackup(context.Background(), site, "job-1", "u1")

	if got := strings.Join(h.calls, ","); got != "backup.create,backup.upload,backup.delete" {
		t.Fatalf("call order = %s", got)
	}
	up := h.args["backup.upload"][0]
	if up["file"] != local || up["host"] != "backup.example.com" || up["port"] != "2222" ||
		up["username"] != "zx_backup" || up["path"] != "/backups/zenex" {
		t.Fatalf("backup.upload args = %v", up)
	}
	if del := h.args["backup.delete"][0]; del["path"] != local || len(del) != 1 {
		t.Fatalf("local archive delete args = %v", del)
	}
	want := "s1|sftp://backup.example.com/backups/zenex/20261009-030000.tar.gz"
	if len(st.inserted) != 1 || st.inserted[0] != want {
		t.Fatalf("recorded %v, want %s", st.inserted, want)
	}
}

func TestFailedSFTPUploadRecordsNothingAndRemovesLocalCopy(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	st.settings = sftpSettings()
	local := BackupRoot + "/zx_shop/x.tar.gz"
	h.outputs["backup.create"] = local + " 4096"
	h.fail["backup.upload"] = errors.New("connection refused")
	site := store.Site{ID: "s1", NodeID: "n1", LinuxUser: "zx_shop", State: "ready", Domain: "shop.example.com"}

	m.runBackup(context.Background(), site, "job-1", "u1")

	if len(st.inserted) != 0 {
		t.Fatalf("record stored after a failed upload: %v", st.inserted)
	}
	if len(h.args["backup.delete"]) != 1 || h.args["backup.delete"][0]["path"] != local {
		t.Fatalf("local archive not removed: %v", h.args["backup.delete"])
	}
}

func TestPruneRemoteArchiveUsesRemoteArguments(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	st.settings = sftpSettings()
	now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	remote := "sftp://backup.example.com/backups/zenex/old.tar.gz"
	st.backups = []store.Backup{{ID: 5, CreatedAt: now.AddDate(0, 0, -10), Path: remote}}

	m.PruneBackups(context.Background(), now, 7)

	args := h.args["backup.delete"]
	if len(args) != 1 {
		t.Fatalf("backup.delete calls = %d", len(args))
	}
	want := map[string]string{"remote": "1", "host": "backup.example.com", "port": "2222", "username": "zx_backup", "path": "/backups/zenex/old.tar.gz"}
	for k, v := range want {
		if args[0][k] != v {
			t.Fatalf("arg %s = %q, want %q (all: %v)", k, args[0][k], v, args[0])
		}
	}
	if _, hasLocal := args[0]["file"]; hasLocal || len(args[0]) != len(want) {
		t.Fatalf("unexpected arguments: %v", args[0])
	}
	if len(st.deleted) != 1 || st.deleted[0] != 5 {
		t.Fatalf("deleted records = %v", st.deleted)
	}
}

func TestFailedRemoteDeleteKeepsRecordForRetry(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	st.settings = sftpSettings()
	now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	st.backups = []store.Backup{{ID: 9, CreatedAt: now.AddDate(0, 0, -30), Path: "sftp://backup.example.com/backups/zenex/x.tar.gz"}}
	h.fail["backup.delete"] = errors.New("sftp: no such file")
	m.PruneBackups(context.Background(), now, 7)
	if len(st.deleted) != 0 {
		t.Fatal("record removed although the remote archive may still exist")
	}
}

func TestMalformedRemoteRecordIsNotDeletedAsLocal(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	st.backups = []store.Backup{{ID: 3, CreatedAt: now.AddDate(0, 0, -30), Path: "sftp://backup.example.com"}}
	m.PruneBackups(context.Background(), now, 7)
	if len(h.args["backup.delete"]) != 0 || len(st.deleted) != 0 {
		t.Fatalf("malformed remote record was acted on: %v %v", h.args, st.deleted)
	}
}

func TestParseRemoteBackupPath(t *testing.T) {
	host, file, remote, err := parseRemoteBackupPath("sftp://backup.example.com/backups/zenex/a.tar.gz")
	if err != nil || !remote || host != "backup.example.com" || file != "/backups/zenex/a.tar.gz" {
		t.Fatalf("parse = %q %q %v %v", host, file, remote, err)
	}
	if _, _, remote, err := parseRemoteBackupPath(BackupRoot + "/zx_a/x.tar.gz"); remote || err != nil {
		t.Fatalf("local path treated as remote: %v %v", remote, err)
	}
	for _, bad := range []string{"sftp://", "sftp:///x", "sftp://host", "sftp://host/a/../../etc"} {
		if _, _, _, err := parseRemoteBackupPath(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestSFTPPublicKeyComesFromHelperAndIsChecked(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	h.outputs["backup.keygen"] = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl zenex-backup\n"
	key, err := m.SFTPPublicKey(context.Background())
	if err != nil || !strings.HasPrefix(key, "ssh-ed25519 AAAA") || strings.Contains(key, "\n") {
		t.Fatalf("key = %q %v", key, err)
	}
	for _, bad := range []string{"", "ssh-rsa AAAAB3NzaC1yc2E zenex", "ssh-ed25519 AAAA!!!! zenex", "ssh-ed25519 AAAAC3 a\nssh-ed25519 AAAAC3 b"} {
		h.outputs["backup.keygen"] = bad
		if _, err := m.SFTPPublicKey(context.Background()); err == nil {
			t.Errorf("accepted helper output %q", bad)
		}
	}
}

func TestSFTPTestSendsSavedDestination(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	if err := m.TestSFTP(context.Background(), testSFTP); err != nil {
		t.Fatal(err)
	}
	args := h.args["backup.test"][0]
	if args["host"] != "backup.example.com" || args["port"] != "2222" || args["username"] != "zx_backup" || args["path"] != "/backups/zenex" {
		t.Fatalf("backup.test args = %v", args)
	}
	if _, has := args["password"]; has {
		t.Fatalf("key auth sent a password: %v", args)
	}
}

const testPassword = "pw-S3cret-42"

// withSFTPPassword makes the manager read the password from a fixed value, as the
// real hook does from the encrypted setting.
func withSFTPPassword(m *Manager, value string) {
	m.SFTPPassword = func(context.Context) (string, error) {
		if value == "" {
			return "", store.ErrNotFound
		}
		return value, nil
	}
}

func passwordDest() store.SFTPDestination {
	d := testSFTP
	d.Auth = store.SFTPAuthPassword
	return d
}

func TestSFTPTestSendsPasswordOnlyForPasswordAuth(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	withSFTPPassword(m, testPassword)
	if err := m.TestSFTP(context.Background(), passwordDest()); err != nil {
		t.Fatal(err)
	}
	if got := h.args["backup.test"][0]["password"]; got != testPassword {
		t.Fatalf("password arg = %q", got)
	}
	if err := m.TestSFTP(context.Background(), testSFTP); err != nil {
		t.Fatal(err)
	}
	if _, has := h.args["backup.test"][1]["password"]; has {
		t.Fatalf("key auth sent a password: %v", h.args["backup.test"][1])
	}
}

func TestSFTPPasswordMissingStopsBeforeHelper(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	withSFTPPassword(m, "")
	if err := m.TestSFTP(context.Background(), passwordDest()); err == nil {
		t.Fatal("test ran without a saved password")
	}
	if len(h.calls) != 0 {
		t.Fatalf("helper called: %v", h.calls)
	}
}

func TestBackupUploadAndRemoteDeleteCarryPasswordOnlyForPasswordAuth(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	withSFTPPassword(m, testPassword)
	st.settings = sftpSettings()
	st.settings.Destination.SFTP.Auth = store.SFTPAuthPassword
	local := BackupRoot + "/zx_shop/20261009-030000.tar.gz"
	h.outputs["backup.create"] = local + " 4096"
	site := store.Site{ID: "s1", NodeID: "n1", LinuxUser: "zx_shop", State: "ready", Domain: "shop.example.com"}

	m.runBackup(context.Background(), site, "job-1", "u1")

	if got := h.args["backup.upload"][0]["password"]; got != testPassword {
		t.Fatalf("upload password arg = %q", got)
	}
	if del := h.args["backup.delete"][0]; len(del) != 1 || del["path"] != local {
		t.Fatalf("local delete carried extra arguments: %v", del)
	}

	now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	st.backups = []store.Backup{{ID: 5, CreatedAt: now.AddDate(0, 0, -10), Path: "sftp://backup.example.com/backups/zenex/old.tar.gz"}}
	m.PruneBackups(context.Background(), now, 7)
	remoteDel := h.args["backup.delete"][1]
	if remoteDel["remote"] != "1" || remoteDel["password"] != testPassword {
		t.Fatalf("remote delete args = %v", remoteDel)
	}
}

func TestPasswordErrorsAreRedacted(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	withSFTPPassword(m, testPassword)
	h.fail["backup.test"] = &helperclient.Error{Message: "login with " + testPassword + " refused"}
	err := m.TestSFTP(context.Background(), passwordDest())
	var helperErr *helperclient.Error
	if !errors.As(err, &helperErr) || strings.Contains(err.Error(), testPassword) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("error = %v", err)
	}

	h.fail["backup.test"] = errors.New("dial failed for " + testPassword)
	if err := m.TestSFTP(context.Background(), passwordDest()); err == nil || strings.Contains(err.Error(), testPassword) {
		t.Fatalf("plain error = %v", err)
	}
}
