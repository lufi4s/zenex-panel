package helper

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

// usageExec answers the database size query with fixed rows and records nothing else.
type usageExec struct {
	sqlOut string
	calls  []string
}

func (u *usageExec) Run(_ context.Context, bin string, args []string, _ time.Duration) (executor.Result, error) {
	u.calls = append(u.calls, bin)
	return executor.Result{}, nil
}

func (u *usageExec) RunInput(_ context.Context, bin string, args []string, _ string, _ time.Duration) (executor.Result, error) {
	u.calls = append(u.calls, bin)
	return executor.Result{Stdout: u.sqlOut}, nil
}

func (u *usageExec) RunEnv(_ context.Context, bin string, args []string, _ []string, _ time.Duration) (executor.Result, error) {
	return u.Run(nil, bin, args, 0)
}

func (u *usageExec) RunToFile(_ context.Context, bin string, args []string, _ []string, _ string, _ time.Duration) (executor.Result, error) {
	return u.Run(nil, bin, args, 0)
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSitesUsageMeasuresFilesDatabaseAndBackups(t *testing.T) {
	ex := &usageExec{sqlOut: "zx_shop\t4096\nzx_blog\t8192\ninformation_schema\t1\n"}
	o, _ := testOps(t, ex)
	writeFile(t, filepath.Join(o.Paths.WebRoot, "zx_shop", "htdocs", "index.php"), 100)
	writeFile(t, filepath.Join(o.Paths.WebRoot, "zx_shop", "htdocs", "wp-content", "a.jpg"), 50)
	writeFile(t, filepath.Join(o.Paths.WebRoot, "zx_blog", "htdocs", "index.php"), 10)
	writeFile(t, filepath.Join(o.Paths.WebRoot, "not-a-site", "index.php"), 999)
	writeFile(t, filepath.Join(o.Paths.BackupDir, "zx_shop", "20261010-030000.tar.gz"), 700)

	res, err := o.Do(context.Background(), "usage.sites", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []SiteUsage
	if err := json.Unmarshal([]byte(res.Output), &got); err != nil {
		t.Fatalf("output is not JSON: %v (%s)", err, res.Output)
	}
	if len(got) != 2 {
		t.Fatalf("usage rows = %+v, want two websites", got)
	}
	// Rows are sorted by account name, so zx_blog comes before zx_shop.
	blog, shop := got[0], got[1]
	if shop.User != "zx_shop" || shop.FilesBytes != 150 || shop.DatabaseBytes != 4096 || shop.BackupBytes != 700 {
		t.Errorf("zx_shop = %+v", shop)
	}
	if blog.User != "zx_blog" || blog.FilesBytes != 10 || blog.DatabaseBytes != 8192 || blog.BackupBytes != 0 {
		t.Errorf("zx_blog = %+v", blog)
	}
	if !strings.Contains(strings.Join(ex.calls, " "), binMariadb) {
		t.Fatal("database size query did not run")
	}
}

func TestSitesUsageIgnoresMalformedDatabaseRows(t *testing.T) {
	ex := &usageExec{sqlOut: "zx_shop\tnot-a-number\nno-tab-here\n"}
	o, _ := testOps(t, ex)
	writeFile(t, filepath.Join(o.Paths.WebRoot, "zx_shop", "index.php"), 5)
	res, err := o.Do(context.Background(), "usage.sites", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, `"database_bytes":0`) {
		t.Fatalf("bad row was used: %s", res.Output)
	}
}
