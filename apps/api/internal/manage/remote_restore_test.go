package manage

import (
	"context"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

func remoteTestSettings() store.BackupSettings {
	return store.BackupSettings{Destination: store.BackupDestination{
		Type: store.BackupDestSFTP,
		SFTP: store.SFTPDestination{Host: "backup.example.com", Port: 22, Username: "zenex", Path: "/backups/zenex", Auth: store.SFTPAuthKey},
	}}
}

func TestListRemoteBackupsSortsNewestFirstAndSkipsBadTimes(t *testing.T) {
	m, fs, h := newFeatureTest("ready")
	fs.settings = remoteTestSettings()
	h.outputs["backup.discover"] = `[
		{"file":"/backups/zenex/zx_a-1.tar.gz","size_bytes":10,"domain":"a.example.com","site_id":"s1","created_at":"2026-10-08T03:00:00Z"},
		{"file":"/backups/zenex/zx_b-2.tar.gz","size_bytes":20,"domain":"b.example.com","site_id":"s2","created_at":"2026-10-09T03:00:00Z"},
		{"file":"/backups/zenex/zx_c-3.tar.gz","size_bytes":30,"domain":"c.example.com","site_id":"s3","created_at":"not a time"}
	]`

	list, err := m.ListRemoteBackups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Domain != "b.example.com" || list[1].Domain != "a.example.com" {
		t.Fatalf("list = %+v", list)
	}
	args := h.args["backup.discover"]
	if len(args) != 1 || args[0]["host"] != "backup.example.com" || args[0]["path"] != "/backups/zenex" {
		t.Fatalf("discover args = %v", args)
	}
}

func TestListRemoteBackupsRefusesLocalDestination(t *testing.T) {
	m, fs, _ := newFeatureTest("ready")
	fs.settings = store.BackupSettings{Destination: store.BackupDestination{Type: store.BackupDestLocal}}
	if _, err := m.ListRemoteBackups(context.Background()); !asRefusal(err) {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

func TestRemoteRestoreSendsSourceDomainAndDatabasePassword(t *testing.T) {
	m, fs, h := newFeatureTest("ready")
	fs.settings = remoteTestSettings()
	m.DBPassword = func(siteID string) string { return "deadbeef" }
	h.outputs["backup.create"] = BackupRoot + "/zx_shop/20261009-100000.tar.gz 1024"
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", DBName: "zx_shop", Domain: "new.example.com", State: "ready"}
	backup := store.Backup{
		SiteID: site.ID, CreatedAt: time.Now(),
		Path: "sftp://backup.example.com/backups/zenex/zx_shop-1.tar.gz",
	}

	if err := m.restoreSite(context.Background(), site, backup, "old.example.com", "job-1"); err != nil {
		t.Fatal(err)
	}
	restore := h.args["backup.restore"]
	if len(restore) != 1 {
		t.Fatalf("backup.restore calls = %d", len(restore))
	}
	got := restore[0]
	if got["dbpass"] != "deadbeef" {
		t.Fatalf("dbpass = %q", got["dbpass"])
	}
	if got["from_domain"] != "old.example.com" || got["to_domain"] != "new.example.com" {
		t.Fatalf("domains = %q -> %q", got["from_domain"], got["to_domain"])
	}
	if got["archive"] == "" || got["archive"] == backup.Path {
		t.Fatalf("archive = %q, want the downloaded local copy", got["archive"])
	}
	if len(h.args["backup.download"]) != 1 {
		t.Fatalf("download calls = %d", len(h.args["backup.download"]))
	}
}

func TestLocalRestoreWithSameDomainChangesNothingAddressRelated(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	m.DBPassword = func(siteID string) string { return "deadbeef" }
	h.outputs["backup.create"] = BackupRoot + "/zx_shop/20261009-100000.tar.gz 1024"
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", DBName: "zx_shop", Domain: "shop.example.com", State: "ready"}
	backup := store.Backup{SiteID: site.ID, Path: BackupRoot + "/zx_shop/20261008-100000.tar.gz"}

	if err := m.restoreSite(context.Background(), site, backup, site.Domain, "job-2"); err != nil {
		t.Fatal(err)
	}
	got := h.args["backup.restore"][0]
	if _, ok := got["from_domain"]; ok {
		t.Fatalf("from_domain sent for the same domain: %v", got)
	}
	if got["archive"] != backup.Path {
		t.Fatalf("archive = %q", got["archive"])
	}
	if len(h.args["backup.download"]) != 0 {
		t.Fatal("a local backup was downloaded")
	}
}
