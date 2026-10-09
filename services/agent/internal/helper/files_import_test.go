package helper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidUploadName(t *testing.T) {
	for _, ok := range []string{"photo.jpg", "my file (1).pdf", "ü.txt"} {
		if !validUploadName(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "bad\x00name", "tab\tname", strings.Repeat("a", 201)} {
		if validUploadName(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestFilesImportRefusesStagedFileOutsideUploadFolder(t *testing.T) {
	root := t.TempDir()
	o := siteFixture(t)
	o.Paths.UploadDir = filepath.Join(root, "uploads")
	outside := filepath.Join(root, "elsewhere.bin")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := o.filesImport(context.Background(), "zx_shop", "", "new.bin", outside)
	if err == nil {
		t.Fatal("a file outside the upload folder was imported")
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatal("a file outside the upload folder was removed")
	}
}

func TestFilesImportRefusesBadNameBeforeTouchingUpload(t *testing.T) {
	o := siteFixture(t)
	o.Paths.UploadDir = t.TempDir()
	staged := filepath.Join(o.Paths.UploadDir, "upload-1")
	if err := os.WriteFile(staged, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := o.filesImport(context.Background(), "zx_shop", "", "../evil.php", staged); err == nil {
		t.Fatal("a name with a folder was accepted")
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatal("staged file removed after a refused name")
	}
}

func TestWPAutoLoginRejectsBadInput(t *testing.T) {
	o := siteFixture(t)
	ctx := context.Background()
	cases := map[string]map[string]string{
		"bad key":   {"user": "zx_shop", "key": "not-hex!", "login": "zenexadmin"},
		"bad login": {"user": "zx_shop", "key": strings.Repeat("ab", 16), "login": "admin'); --"},
		"bad user":  {"user": "root", "key": strings.Repeat("ab", 16), "login": "zenexadmin"},
	}
	for name, args := range cases {
		if err := o.wpAutoLogin(ctx, args); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := os.Stat(filepath.Join(o.docRoot("zx_shop"), "wp-content", "mu-plugins")); err == nil {
		t.Fatal("the plugin folder was created for rejected input")
	}
}
