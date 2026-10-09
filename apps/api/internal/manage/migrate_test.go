package manage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// GetJob is the default for tests that do not run a migration: no job is known.
func (f *fakeStore) GetJob(context.Context, string) (store.Job, error) {
	return store.Job{}, store.ErrNotFound
}

// migrationStore adds the build job's status to the feature fakes.
type migrationStore struct {
	*featureStore
	jobs   map[string]store.Job
	logs   []string
	notes  []string
	finish string
}

func (s *migrationStore) GetJob(_ context.Context, id string) (store.Job, error) {
	job, ok := s.jobs[id]
	if !ok {
		return store.Job{}, store.ErrNotFound
	}
	return job, nil
}

func (s *migrationStore) AppendJobLog(_ context.Context, _, _, msg string) error {
	s.logs = append(s.logs, msg)
	return nil
}

func (s *migrationStore) FinishJob(_ context.Context, _, status, msg string) error {
	s.finish = status + ":" + msg
	return nil
}

func (s *migrationStore) Notify(_ context.Context, _, level, title, body string) error {
	s.notes = append(s.notes, level+"|"+title+"|"+body)
	return nil
}

func newMigrationTest(t *testing.T) (*Manager, *migrationStore, *scriptedHelper) {
	t.Helper()
	migratePoll = time.Millisecond
	t.Cleanup(func() { migratePoll = 3 * time.Second })
	fs := &featureStore{fakeStore: &fakeStore{state: "ready"}, maintenance: map[string]bool{}}
	st := &migrationStore{featureStore: fs, jobs: map[string]store.Job{}}
	h := &scriptedHelper{outputs: map[string]string{}, fail: map[string]error{}}
	m := New(st, h, quietLogger())
	m.DBPassword = func(string) string { return "deadbeef" }
	m.PublicIP = "203.0.113.7"
	h.outputs["cpanel.pull"] = BackupRoot + "/zx_shop/migrate-20261009-100000.tar.gz 4096"
	h.outputs["wp.verify"] = "https://www.example.com"
	return m, st, h
}

func migrationRequest(provisionJob string) MigrationRequest {
	return MigrationRequest{
		Site:           store.Site{ID: "s1", LinuxUser: "zx_shop", DBName: "zx_shop", Domain: "shop.example.net", State: "ready"},
		ProvisionJobID: provisionJob,
		Creds:          CpanelCreds{Host: "cpanel.example.com", Port: 22, Username: "acct", Password: "hunter2-secret"},
		SourcePath:     "/home/acct/public_html",
		SourceDomain:   "www.example.com",
		ActorID:        "u1",
	}
}

func TestMigrationRunsAllStepsAndChangesTheDomain(t *testing.T) {
	m, st, h := newMigrationTest(t)
	st.jobs["build-1"] = store.Job{ID: "build-1", Status: "succeeded"}

	m.runMigration(context.Background(), migrationRequest("build-1"), "job-1")

	if !strings.HasPrefix(st.finish, "succeeded") {
		t.Fatalf("job ended as %q", st.finish)
	}
	pull := h.args["cpanel.pull"]
	if len(pull) != 1 || pull[0]["path"] != "/home/acct/public_html" || pull[0]["user"] != "zx_shop" || pull[0]["host"] != "cpanel.example.com" {
		t.Fatalf("cpanel.pull args = %v", pull)
	}
	if !strings.HasPrefix(pull[0]["output"], BackupRoot+"/zx_shop/migrate-") {
		t.Fatalf("archive path = %q", pull[0]["output"])
	}
	restore := h.args["backup.restore"]
	if len(restore) != 1 || restore[0]["from_domain"] != "www.example.com" || restore[0]["to_domain"] != "shop.example.net" || restore[0]["dbpass"] != "deadbeef" {
		t.Fatalf("backup.restore args = %v", restore)
	}
	if restore[0]["archive"] != BackupRoot+"/zx_shop/migrate-20261009-100000.tar.gz" {
		t.Fatalf("restored archive = %q", restore[0]["archive"])
	}
	if len(h.args["wp.verify"]) != 1 {
		t.Fatal("the website was not verified")
	}
	if len(st.inserted) != 1 {
		t.Fatalf("the copy was not listed as a backup: %v", st.inserted)
	}
	if len(st.notes) != 1 || !strings.Contains(st.notes[0], "203.0.113.7") || !strings.HasPrefix(st.notes[0], "success|") {
		t.Fatalf("notification = %v", st.notes)
	}
}

func TestMigrationWithSameDomainChangesNoAddresses(t *testing.T) {
	m, st, h := newMigrationTest(t)
	req := migrationRequest("")
	req.SourceDomain = req.Site.Domain

	m.runMigration(context.Background(), req, "job-1")

	if !strings.HasPrefix(st.finish, "succeeded") {
		t.Fatalf("job ended as %q", st.finish)
	}
	if _, ok := h.args["backup.restore"][0]["from_domain"]; ok {
		t.Fatal("from_domain sent for the same domain")
	}
}

func TestMigrationStopsWhenTheNewWebsiteFailsToBuild(t *testing.T) {
	m, st, h := newMigrationTest(t)
	st.jobs["build-1"] = store.Job{ID: "build-1", Status: "failed", Error: "no space left"}

	m.runMigration(context.Background(), migrationRequest("build-1"), "job-1")

	if !strings.HasPrefix(st.finish, "failed:") || !strings.Contains(st.finish, "no space left") {
		t.Fatalf("job ended as %q", st.finish)
	}
	if len(h.args["cpanel.pull"]) != 0 {
		t.Fatal("the download started although the website was not built")
	}
}

func TestMigrationFailureNeverShowsThePassword(t *testing.T) {
	m, st, h := newMigrationTest(t)
	h.fail["cpanel.pull"] = errors.New("remote said: bad login hunter2-secret")

	m.runMigration(context.Background(), migrationRequest(""), "job-1")

	if !strings.HasPrefix(st.finish, "failed:") {
		t.Fatalf("job ended as %q", st.finish)
	}
	all := st.finish + strings.Join(st.logs, "|") + strings.Join(st.notes, "|")
	if strings.Contains(all, "hunter2-secret") {
		t.Fatalf("the password leaked: %s", all)
	}
	if len(h.args["backup.restore"]) != 0 {
		t.Fatal("restore ran after a failed download")
	}
}

func TestMigrationRemovesTheCopyWhenTheRestoreFails(t *testing.T) {
	m, st, h := newMigrationTest(t)
	h.fail["backup.restore"] = errors.New("database restore failed")

	m.runMigration(context.Background(), migrationRequest(""), "job-1")

	if !strings.HasPrefix(st.finish, "failed:") {
		t.Fatalf("job ended as %q", st.finish)
	}
	del := h.args["backup.delete"]
	if len(del) != 1 || del[0]["path"] != BackupRoot+"/zx_shop/migrate-20261009-100000.tar.gz" {
		t.Fatalf("backup.delete args = %v", del)
	}
	if len(h.args["wp.verify"]) != 0 {
		t.Fatal("verification ran after a failed restore")
	}
}

func TestScanCpanelParsesInstallsAndHidesThePassword(t *testing.T) {
	m, _, h := newMigrationTest(t)
	h.outputs["cpanel.scan"] = `[{"path":"/home/acct/public_html","site_url":"https://example.com","domain":"example.com","db_name":"acct_wp","table_prefix":"wp_","size_kb":10}]`

	list, err := m.ScanCpanel(context.Background(), CpanelCreds{Host: "h.example.com", Port: 22, Username: "acct", Password: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Domain != "example.com" || list[0].SizeKB != 10 {
		t.Fatalf("list = %+v", list)
	}

	h.fail["cpanel.scan"] = errors.New("login failed for pw")
	if _, err := m.ScanCpanel(context.Background(), CpanelCreds{Host: "h", Port: 22, Username: "acct", Password: "pw"}); err == nil || strings.Contains(err.Error(), "for pw") {
		t.Fatalf("err = %v", err)
	}
}
