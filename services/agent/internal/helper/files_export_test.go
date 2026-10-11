package helper

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// exportFixture builds a site with a few files and a staged, empty output file.
func exportFixture(t *testing.T) (*Ops, string) {
	t.Helper()
	o := siteFixture(t)
	o.Paths.UploadDir = t.TempDir()
	doc := o.docRoot("zx_shop")
	for rel, body := range map[string]string{
		"index.php":                "<?php",
		"wp-content/a.txt":         "alpha",
		"wp-content/sub/b.txt":     "bravo",
		"wp-content/uploads/c.png": "png",
	} {
		p := filepath.Join(doc, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	staged := filepath.Join(o.Paths.UploadDir, "download-1")
	if err := os.WriteFile(staged, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return o, staged
}

func zipNames(t *testing.T, path string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("not a zip: %v", err)
	}
	defer zr.Close()
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		buf := make([]byte, 64)
		n, _ := rc.Read(buf)
		_ = rc.Close()
		out[f.Name] = string(buf[:n])
	}
	return out
}

func decodeExport(t *testing.T, res Result) exportResult {
	t.Helper()
	var r exportResult
	if err := json.Unmarshal([]byte(res.Output), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestExportOneFileIsCopiedAsItIs(t *testing.T) {
	o, staged := exportFixture(t)
	res, err := o.filesExport(context.Background(), "zx_shop", "wp-content/a.txt", staged)
	if err != nil {
		t.Fatal(err)
	}
	if r := decodeExport(t, res); r.Kind != "file" || r.Name != "a.txt" {
		t.Fatalf("result = %+v", r)
	}
	if got, _ := os.ReadFile(staged); string(got) != "alpha" {
		t.Fatalf("content = %q", got)
	}
}

func TestExportFolderIsAnArchiveWithEverythingInside(t *testing.T) {
	o, staged := exportFixture(t)
	res, err := o.filesExport(context.Background(), "zx_shop", "wp-content", staged)
	if err != nil {
		t.Fatal(err)
	}
	if r := decodeExport(t, res); r.Kind != "zip" || r.Name != "wp-content.zip" {
		t.Fatalf("result = %+v", r)
	}
	got := zipNames(t, staged)
	for name, body := range map[string]string{
		"wp-content/a.txt": "alpha", "wp-content/sub/b.txt": "bravo", "wp-content/uploads/c.png": "png",
	} {
		if got[name] != body {
			t.Errorf("%s = %q, want %q (have %v)", name, got[name], body, got)
		}
	}
}

func TestExportSeveralItemsAreOneArchive(t *testing.T) {
	o, staged := exportFixture(t)
	res, err := o.filesExport(context.Background(), "zx_shop", "index.php\nwp-content/sub", staged)
	if err != nil {
		t.Fatal(err)
	}
	if r := decodeExport(t, res); r.Kind != "zip" || r.Name != "download.zip" {
		t.Fatalf("result = %+v", r)
	}
	got := zipNames(t, staged)
	if got["index.php"] != "<?php" || got["sub/b.txt"] != "bravo" || len(got) != 2 {
		t.Fatalf("archive = %v", got)
	}
}

func TestExportRefusesPathsOutsideTheSiteAndForeignStagedFiles(t *testing.T) {
	o, staged := exportFixture(t)
	for _, bad := range []string{"../zx_other", "../../etc/passwd", "nope.txt", ""} {
		if _, err := o.filesExport(context.Background(), "zx_shop", bad, staged); err == nil && bad != "" {
			t.Errorf("path %q accepted", bad)
		}
	}
	outside := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := o.filesExport(context.Background(), "zx_shop", "index.php", outside); err == nil {
		t.Fatal("an output outside the upload folder was written")
	}
	if _, err := o.filesExport(context.Background(), "zx_shop", strings.Repeat("a\n", 201), staged); err == nil {
		t.Fatal("more than 200 items accepted")
	}
}

func TestExportNeverFollowsLinksOutOfTheSite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses symbolic links, POSIX only")
	}
	o, staged := exportFixture(t)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(o.docRoot("zx_shop"), "wp-content", "link.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := o.filesExport(context.Background(), "zx_shop", "wp-content/link.txt", staged); err == nil {
		t.Fatal("a link out of the site was downloaded")
	}
	if _, err := o.filesExport(context.Background(), "zx_shop", "wp-content", staged); err != nil {
		t.Fatal(err)
	}
	for name, body := range zipNames(t, staged) {
		if strings.Contains(body, "top secret") || strings.HasSuffix(name, "link.txt") {
			t.Fatalf("archive contains the linked file: %s", name)
		}
	}
}
