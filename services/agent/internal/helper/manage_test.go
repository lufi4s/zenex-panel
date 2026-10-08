package helper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogsTailReturnsLastLines(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("line-")
		b.WriteString(strings.Repeat("x", 3))
		b.WriteString("\n")
	}
	b.WriteString("final error line\n")
	if err := os.WriteFile(filepath.Join(dir, "zx-zx_shop.error.log"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	o := &Ops{Paths: Paths{LogDir: dir}}
	res, err := o.logsTail(map[string]string{"user": "zx_shop"})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(res.Output, "\n")
	if len(lines) != logTailLines {
		t.Fatalf("got %d lines, want %d", len(lines), logTailLines)
	}
	if lines[len(lines)-1] != "final error line" {
		t.Fatalf("last line = %q", lines[len(lines)-1])
	}
}

func TestLogsTailWithoutLogFile(t *testing.T) {
	o := &Ops{Paths: Paths{LogDir: t.TempDir()}}
	res, err := o.logsTail(map[string]string{"user": "zx_new"})
	if err != nil || res.Output == "" {
		t.Fatalf("expected a friendly empty message, got %q, %v", res.Output, err)
	}
}

func TestPurgeRequiresMatchingIdentifiers(t *testing.T) {
	f := &fakeExec{}
	o := &Ops{Exec: f, Paths: DefaultPaths()}
	err := o.sitePurge(context.Background(), map[string]string{"user": "zx_shop", "db": "zx_other", "dbuser": "zx_shop"})
	if err == nil {
		t.Fatal("purge accepted mismatched database name")
	}
	if len(f.calls) != 0 {
		t.Fatalf("commands ran before validation: %v", f.calls)
	}
}

func TestPHPVersionsFromInstalledBinaries(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"php-fpm8.3", "php-fpm8.4", "php-fpm-other"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	o := &Ops{Paths: Paths{PHPFPMGlob: filepath.Join(dir, "php-fpm[0-9]*.[0-9]*")}}
	got := o.phpVersions()
	if len(got) != 2 || got[0] != "8.3" || got[1] != "8.4" {
		t.Fatalf("versions = %v", got)
	}
}
