package executor

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// testBin returns an absolute path under the platform root, e.g. /usr/sbin/nginx
// on Linux and C:\usr\sbin\nginx on Windows.
func testBin(parts ...string) string {
	root := string(filepath.Separator)
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	return filepath.Join(append([]string{root}, parts...)...)
}

func TestNewRunnerRejectsRelativeAndDirtyPaths(t *testing.T) {
	for _, p := range []string{"nginx", "bin/nginx", "/usr/bin/../bin/nginx", ""} {
		if _, err := NewRunner(p); err == nil {
			t.Errorf("NewRunner(%q) accepted an unsafe path", p)
		}
	}
}

func TestRunRejectsNonAllowlistedBinary(t *testing.T) {
	r, err := NewRunner(testBin("usr", "sbin", "nginx"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Run(context.Background(), testBin("bin", "sh"), []string{"-c", "id"}, time.Second)
	if !errors.Is(err, ErrBinaryNotAllowed) {
		t.Fatalf("expected ErrBinaryNotAllowed, got %v", err)
	}
}

func TestRunRejectsControlCharacters(t *testing.T) {
	bin := testBin("bin", "echo")
	r, err := NewRunner(bin)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range []string{"a\nb", "a\x00b", "a\rb"} {
		if _, err := r.Run(context.Background(), bin, []string{arg}, time.Second); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("arg %q: expected ErrInvalidArgument, got %v", arg, err)
		}
	}
}

func TestRunRequiresPositiveTimeout(t *testing.T) {
	bin := testBin("bin", "echo")
	r, err := NewRunner(bin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), bin, nil, 0); err == nil {
		t.Fatal("zero timeout accepted")
	}
}

func TestRunExecutesAllowlistedBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX binaries; run on the Linux build target")
	}
	bin := "/bin/echo"
	r, _ := NewRunner(bin)
	res, err := r.Run(context.Background(), bin, []string{"hello"}, 5*time.Second)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.ExitCode != 0 || res.Stdout != "hello\n" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunReportsTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX binaries; run on the Linux build target")
	}
	bin := "/bin/sleep"
	r, _ := NewRunner(bin)
	_, err := r.Run(context.Background(), bin, []string{"5"}, 100*time.Millisecond)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}
