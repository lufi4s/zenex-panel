package helper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// siteFixture builds /<root>/zx_shop/htdocs with a few files and returns the ops.
func siteFixture(t *testing.T) *Ops {
	t.Helper()
	root := t.TempDir()
	doc := filepath.Join(root, "zx_shop", "htdocs")
	if err := os.MkdirAll(filepath.Join(doc, "wp-content"), 0o750); err != nil {
		t.Fatal(err)
	}
	must := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	must(filepath.Join(doc, "index.php"), "<?php echo 1;")
	must(filepath.Join(doc, "wp-config.php"), "<?php // secrets")
	must(filepath.Join(doc, "wp-content", "notes.txt"), "hello")
	must(filepath.Join(doc, "image.bin"), "bin\x00ary")
	return &Ops{Paths: Paths{WebRoot: root}}
}

func TestPathsCannotLeaveTheSiteFolder(t *testing.T) {
	o := siteFixture(t)
	for _, bad := range []string{"../zx_other/htdocs/x", "..", "wp-content/../../x", "a\x00b"} {
		if _, err := o.resolveInSite("zx_shop", bad); err == nil {
			t.Errorf("path %q was accepted", bad)
		}
	}
}

func TestSymlinkCannotPointOutOfTheSite(t *testing.T) {
	o := siteFixture(t)
	outside := t.TempDir()
	link := filepath.Join(o.docRoot("zx_shop"), "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic links unavailable here: %v", err)
	}
	if _, err := o.resolveInSite("zx_shop", "escape/secret.txt"); err == nil {
		t.Fatal("a symbolic link led outside the website folder")
	}
}

func TestListShowsFoldersFirst(t *testing.T) {
	o := siteFixture(t)
	res, err := o.filesList("zx_shop", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, `"name":"wp-content","type":"dir"`) {
		t.Fatalf("folder missing from listing: %s", res.Output)
	}
	if strings.Index(res.Output, "wp-content") > strings.Index(res.Output, "index.php") {
		t.Fatal("folders should come before files")
	}
}

func TestBinaryFilesCannotBeReadAsText(t *testing.T) {
	o := siteFixture(t)
	if _, err := o.filesRead("zx_shop", "image.bin"); err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("binary file was returned as text: %v", err)
	}
	res, err := o.filesRead("zx_shop", "wp-content/notes.txt")
	if err != nil || res.Output != "hello" {
		t.Fatalf("text read failed: %q, %v", res.Output, err)
	}
}

func TestWordPressCoreFilesAreProtected(t *testing.T) {
	o := siteFixture(t)
	if err := o.filesDelete("zx_shop", "wp-config.php"); err == nil {
		t.Fatal("wp-config.php could be deleted")
	}
	if err := o.filesDelete("zx_shop", ""); err == nil {
		t.Fatal("the whole website folder could be deleted")
	}
	if err := o.filesDelete("zx_shop", "wp-content/notes.txt"); err != nil {
		t.Fatalf("ordinary file delete failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(o.docRoot("zx_shop"), "wp-content", "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("file was not removed")
	}
}

func TestFileOperationsRequireAValidAccountName(t *testing.T) {
	o := siteFixture(t)
	if _, err := o.filesOp(nil, "files.list", map[string]string{"user": "root", "path": ""}); err == nil {
		t.Fatal("system account accepted for file access")
	}
}

// A leading slash means "from the site root", so it stays inside the site.
func TestLeadingSlashStaysInsideTheSite(t *testing.T) {
	o := siteFixture(t)
	full, err := o.resolveInSite("zx_shop", "/etc/passwd")
	if err != nil {
		t.Fatalf("rejected: %v", err)
	}
	if !strings.HasPrefix(full, filepath.Clean(o.docRoot("zx_shop"))+string(filepath.Separator)) {
		t.Fatalf("resolved outside the site: %s", full)
	}
}
