package helper

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

const sampleListing = `sftp> ls -l
-rw-r--r--    1 zenex    zenex        1234 Oct 09 10:00 zx_shop-20261009-100000.tar.gz
-rw-r--r--    1 zenex    zenex         312 Oct 09 10:00 zx_shop-20261009-100000.json
drwxr-xr-x    2 zenex    zenex        4096 Oct 09 10:00 archive-dir
-rw-r--r--    1 zenex    zenex         500 Oct 09 10:00 orphan.json
`

func TestParseSFTPListingReadsRegularFilesOnly(t *testing.T) {
	files := parseSFTPListing(sampleListing)
	if got := files["zx_shop-20261009-100000.tar.gz"]; got != 1234 {
		t.Fatalf("archive size = %d", got)
	}
	if _, ok := files["archive-dir"]; ok {
		t.Fatal("directory listed as a file")
	}
	if _, ok := files["sftp>"]; ok {
		t.Fatal("command echo treated as a file")
	}
}

func TestArchivesWithManifestNeedsBothFiles(t *testing.T) {
	got := archivesWithManifest(parseSFTPListing(sampleListing))
	if len(got) != 1 || got[0] != "zx_shop-20261009-100000.tar.gz" {
		t.Fatalf("offered = %v", got)
	}
}

func TestReadManifestRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	valid := `{"format":1,"domain":"shop.example.com","site_id":"11111111-2222-3333-4444-555555555555","linux_user":"zx_shop","database":"zx_shop","created_at":"2026-10-09T10:00:00Z"}`
	if m, ok := readManifest(write("ok.json", valid)); !ok || m.Domain != "shop.example.com" {
		t.Fatalf("valid manifest rejected: %+v %v", m, ok)
	}
	cases := map[string]string{
		"not json":    `{"domain":`,
		"other fmt":   `{"format":2,"domain":"shop.example.com"}`,
		"no domain":   `{"format":1,"site_id":"11111111-2222-3333-4444-555555555555"}`,
		"bad domain":  `{"format":1,"domain":"../etc","site_id":"11111111-2222-3333-4444-555555555555"}`,
		"bad site id": `{"format":1,"domain":"shop.example.com","site_id":"x"}`,
	}
	for name, body := range cases {
		if _, ok := readManifest(write(name+".json", body)); ok {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, ok := readManifest(filepath.Join(dir, "missing.json")); ok {
		t.Fatal("missing manifest accepted")
	}
}

func TestBackupDiscoverRejectsUnsafeDirectoryBeforeSFTP(t *testing.T) {
	ex := &recExec{}
	o, keyDir := remoteOps(t, ex)
	withKey(t, keyDir)
	args := map[string]string{
		"host": "backup.example.com", "port": "22", "username": "zenex", "path": "/backups/../etc",
	}
	if _, err := o.Do(context.Background(), "backup.discover", args); err == nil {
		t.Fatal("discover accepted an unsafe directory")
	}
	if len(ex.calls) != 0 {
		t.Fatalf("sftp ran for an unsafe directory: %v", ex.calls)
	}
}
