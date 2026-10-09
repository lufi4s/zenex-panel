package store

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/migrations"
)

// openTestStore connects to the database named by ZENEX_TEST_DATABASE_URL and applies the
// migrations. The tests are skipped when it is not set. Use an empty, throw-away database:
//
//	createdb zx_test && ZENEX_TEST_DATABASE_URL=postgres://localhost/zx_test go test ./internal/store
func openTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("ZENEX_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ZENEX_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.Migrate(ctx, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

// fixtureSite creates an owner, a node, a connected domain and one website.
func fixtureSite(t *testing.T, s *Store, label string) (ownerID, nodeID, siteID string) {
	t.Helper()
	ctx := context.Background()
	// A suffix makes every run use new names, so the tests can run again on the same database.
	label += strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 36)
	// The local node needs an administrator to exist. A second run finds it already there.
	_, _ = s.CreateUser(ctx, "fixture-admin@example.com", "hash", "administrator")
	ownerID, err := s.CreateUser(ctx, label+"@example.com", "hash", "customer")
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err = s.EnsureLocalNode(ctx, "test-node", "203.0.113.1")
	if err != nil {
		t.Fatal(err)
	}
	dom, err := s.CreateDomain(ctx, ownerID, label+".example.com")
	if err != nil {
		t.Fatal(err)
	}
	zoneID, err := s.DomainOwnedBy(ctx, ownerID, dom.Apex)
	if err != nil {
		t.Fatal(err)
	}
	siteID, _, _, err = s.CreateSiteWithJob(ctx, NewSite{
		OwnerID: ownerID, NodeID: nodeID, ZoneID: zoneID, Slug: "www-" + label, Domain: "www." + label + ".example.com",
		LinuxUser: "zx_www_" + label, DBName: "zx_www_" + label, DBUser: "zx_www_" + label,
		PHPVersion: "8.3", IdempotencyKey: "key-" + label + "-12345", JobType: "site.provision",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ownerID, nodeID, siteID
}

func TestEnsureLocalNodeWithoutAPublicIP(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	_, _ = s.CreateUser(ctx, "fixture-admin@example.com", "hash", "administrator")

	id, err := s.EnsureLocalNode(ctx, "node-without-ip", "")
	if err != nil || id == "" {
		t.Fatalf("a node without a public IP was refused: %q, %v", id, err)
	}
	again, err := s.EnsureLocalNode(ctx, "node-without-ip", "")
	if err != nil || again != id {
		t.Fatalf("second call = %q, %v (want %q)", again, err, id)
	}
}

var activityTypes = []string{"site.backup", "site.restore", "site.migrate", "site.provision", "site.delete"}

func TestSiteActivityAgainstPostgres(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	owner, node, site := fixtureSite(t, s, "activity")

	// The build job created with the website is queued, so it is reported.
	got, err := s.ListSiteActivity(ctx, owner, activityTypes, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SiteID != site || got[0].Type != "site.provision" || got[0].Status != "queued" {
		t.Fatalf("activity = %+v", got)
	}

	// A running backup with steps: the newest job of the website wins, and its steps come in order.
	jobID, err := s.CreateManagementJob(ctx, owner, site, node, "site.backup")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureJobSteps(ctx, jobID, []string{"archive", "upload", "record"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStepStatus(ctx, jobID, "archive", "succeeded", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStepStatus(ctx, jobID, "upload", "running", ""); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListSiteActivity(ctx, owner, activityTypes, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].JobID != jobID || got[0].Status != "running" || len(got[0].Steps) != 3 {
		t.Fatalf("activity = %+v", got)
	}
	if got[0].Steps[0].Name != "archive" || got[0].Steps[1].Status != "running" || got[0].Steps[2].Status != "pending" {
		t.Fatalf("steps = %+v", got[0].Steps)
	}

	// Another owner sees nothing; an empty owner ("administrator") sees everything.
	other, err := s.ListSiteActivity(ctx, "00000000-0000-0000-0000-000000000001", activityTypes, 10*time.Minute)
	if err != nil || len(other) != 0 {
		t.Fatalf("other owner = %+v, %v", other, err)
	}
	all, err := s.ListSiteActivity(ctx, "", activityTypes, 10*time.Minute)
	if err != nil || len(all) == 0 {
		t.Fatalf("all = %+v, %v", all, err)
	}

	// A finished job stays visible for the recent window only.
	if err := s.FinishJob(ctx, jobID, "succeeded", ""); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListSiteActivity(ctx, owner, activityTypes, 10*time.Minute)
	if err != nil || len(got) != 1 || got[0].Status != "succeeded" {
		t.Fatalf("just finished = %+v, %v", got, err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE jobs SET finished_at = now() - interval '1 hour', updated_at = now() - interval '1 hour' WHERE id = $1::uuid`, jobID); err != nil {
		t.Fatal(err)
	}
	// The queued build job is older than the backup but is the only one still reported.
	got, err = s.ListSiteActivity(ctx, owner, activityTypes, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Type == "site.backup" {
		t.Fatalf("an old finished backup is still reported: %+v", got)
	}
}

func TestFailInterruptedJobsAndActiveBackupsAgainstPostgres(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	owner, node, site := fixtureSite(t, s, "interrupt")

	backup, err := s.CreateManagementJob(ctx, owner, site, node, "site.backup")
	if err != nil {
		t.Fatal(err)
	}
	restore, err := s.CreateManagementJob(ctx, owner, site, node, "site.restore")
	if err != nil {
		t.Fatal(err)
	}
	active, err := s.SitesWithActiveBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range active {
		found = found || id == site
	}
	if !found {
		t.Fatalf("a running backup or restore is not reported as active: %v", active)
	}

	if err := s.FailInterruptedJobs(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{backup, restore} {
		job, err := s.GetJob(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != "failed" || job.Error == "" {
			t.Fatalf("job %s = %+v", id, job)
		}
	}
	// The build job (provisioning) is not touched: it is resumed, not failed.
	build, err := s.LatestJobForSiteOfType(ctx, site, "site.provision")
	if err != nil || build.Status != "queued" {
		t.Fatalf("build job = %+v, %v", build, err)
	}
	active, err = s.SitesWithActiveBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range active {
		if id == site {
			t.Fatal("a failed job still blocks the website")
		}
	}
}

func TestBackupRecordsAgainstPostgres(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	_, _, site := fixtureSite(t, s, "records")

	if err := s.InsertBackup(ctx, site, 1234, "/var/backups/zenex/zx_www_records/20261010-010101.tar.gz"); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListBackups(ctx, site)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	one, err := s.GetBackup(ctx, site, list[0].ID)
	if err != nil || one.SizeBytes != 1234 {
		t.Fatalf("backup = %+v, %v", one, err)
	}
	// A backup id of another website must not be readable through this one.
	if _, err := s.GetBackup(ctx, "00000000-0000-0000-0000-000000000009", list[0].ID); err == nil {
		t.Fatal("a backup was returned for the wrong website")
	}
	if _, err := s.GetBackup(ctx, site, list[0].ID+9999); err != ErrNotFound {
		t.Fatalf("missing backup error = %v", err)
	}
}
