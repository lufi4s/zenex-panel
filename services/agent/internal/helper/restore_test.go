package helper

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

func restoreArgs(o *Ops, archive string) map[string]string {
	return map[string]string{
		"user":     "zx_shop",
		"database": "zx_shop",
		"docroot":  o.docRoot("zx_shop"),
		"archive":  archive,
	}
}

func TestBackupRestoreRefusesArchiveOutsideSiteFolder(t *testing.T) {
	ex := &scriptExec{}
	o, root := testOps(t, ex)
	outside := filepath.Join(root, "backups", "zx_other", "shop-1.tar.gz")

	if err := o.backupRestore(context.Background(), restoreArgs(o, outside)); err == nil {
		t.Fatal("restore accepted an archive from another site's folder")
	}
	if len(ex.calls) != 0 {
		t.Fatalf("commands ran before the path check: %v", ex.calls)
	}
}

func TestBackupRestoreRefusesUnexpectedEntries(t *testing.T) {
	ex := &scriptExec{onRun: func(string, []string) (executor.Result, error) {
		return executor.Result{Stdout: "htdocs/index.php\nhtdocs/../../etc/passwd\n"}, nil
	}}
	o, root := testOps(t, ex)
	archive := filepath.Join(root, "backups", "zx_shop", "shop-1.tar.gz")
	if err := os.MkdirAll(filepath.Dir(archive), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := o.backupRestore(context.Background(), restoreArgs(o, archive)); err == nil {
		t.Fatal("restore accepted an archive with unsafe entries")
	}
	if len(ex.calls) != 1 || ex.calls[0] != binTar {
		t.Fatalf("expected only the archive listing to run, got %v", ex.calls)
	}
}

func TestBackupDownloadRefusesUnsafeRemotePathBeforeSFTP(t *testing.T) {
	ex := &scriptExec{}
	o, root := testOps(t, ex)
	local := filepath.Join(root, "backups", "zx_shop", "restore-1.tar.gz")
	args := map[string]string{
		"user":     "zx_shop",
		"host":     "backup.example.com",
		"port":     "22",
		"username": "zenex",
		"path":     "/backups/../other.tar.gz",
		"file":     local,
	}

	if err := o.backupDownload(context.Background(), args); err == nil {
		t.Fatal("download accepted a remote path with ..")
	}
	if len(ex.calls) != 0 {
		t.Fatalf("sftp ran for an invalid path: %v", ex.calls)
	}
}
