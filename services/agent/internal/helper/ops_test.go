package helper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

// fakeExec records every command and reports success unless the binary is in fail.
type fakeExec struct {
	calls []string
	stdin string
	fail  map[string]bool
}

func (f *fakeExec) Run(_ context.Context, bin string, args []string, _ time.Duration) (executor.Result, error) {
	return f.record(bin, args, "")
}

func (f *fakeExec) RunInput(_ context.Context, bin string, args []string, stdin string, _ time.Duration) (executor.Result, error) {
	return f.record(bin, args, stdin)
}

func (f *fakeExec) record(bin string, args []string, stdin string) (executor.Result, error) {
	f.calls = append(f.calls, bin+" "+strings.Join(args, " "))
	f.stdin = stdin
	if f.fail[bin] {
		return executor.Result{ExitCode: 1, Stderr: "boom"}, nil
	}
	return executor.Result{}, nil
}

func validSecret() string { return strings.Repeat("ab", 16) }

func TestValidators(t *testing.T) {
	for _, u := range []string{"zx_shop", "zx_my_shop_2"} {
		if !linuxUserRe.MatchString(u) {
			t.Errorf("valid account %q rejected", u)
		}
	}
	for _, u := range []string{"shop", "root", "zx_", "zx_Shop", "zx_a;rm", "zx_" + strings.Repeat("a", 29)} {
		if linuxUserRe.MatchString(u) {
			t.Errorf("invalid account %q accepted", u)
		}
	}
	if err := requirePHPVersion("8.3"); err != nil {
		t.Error(err)
	}
	for _, v := range []string{"8", "8.3.1", "8.3;rm", "../8.3"} {
		if requirePHPVersion(v) == nil {
			t.Errorf("PHP version %q accepted", v)
		}
	}
	if requireHex("zz", "x") == nil {
		t.Error("non-hex secret accepted")
	}
}

func TestDoRejectsUnknownOperation(t *testing.T) {
	o := &Ops{Exec: &fakeExec{}, Paths: DefaultPaths()}
	if _, err := o.Do(context.Background(), "shell.exec", nil); err == nil {
		t.Fatal("unknown operation accepted")
	}
}

func TestDoRejectsSystemAccountBeforeAnyCommand(t *testing.T) {
	f := &fakeExec{}
	o := &Ops{Exec: f, Paths: DefaultPaths()}
	if _, err := o.Do(context.Background(), "user.create", map[string]string{"user": "root"}); err == nil {
		t.Fatal("system account name accepted")
	}
	if len(f.calls) != 0 {
		t.Fatalf("command ran despite invalid input: %v", f.calls)
	}
}

func TestDBCreateSendsSQLOnStdinNotArgv(t *testing.T) {
	f := &fakeExec{}
	o := &Ops{Exec: f, Paths: DefaultPaths()}
	err := o.dbCreate(context.Background(), map[string]string{
		"db": "zx_shop", "dbuser": "zx_shop", "password": validSecret(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("expected one call, got %d", len(f.calls))
	}
	if strings.Contains(f.calls[0], validSecret()) {
		t.Fatal("password appeared in argv")
	}
	if !strings.Contains(f.stdin, "GRANT ALL PRIVILEGES ON `zx_shop`.*") {
		t.Fatalf("grant missing from SQL: %s", f.stdin)
	}
}

func TestPoolConfigIsolatesSite(t *testing.T) {
	cfg := poolConfig("zx_shop", "/var/www/zx_shop")
	for _, want := range []string{
		"user = zx_shop", "group = zx_shop",
		"listen = /run/php/zx-zx_shop.sock",
		"open_basedir] = /var/www/zx_shop:/tmp/",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("pool config missing %q", want)
		}
	}
}

func TestVhostBlocksPHPInUploads(t *testing.T) {
	cfg := vhostConfig("zx_shop", "shop.example.com", "/var/www/zx_shop/htdocs", "/var/log/nginx")
	if !strings.Contains(cfg, `location ~* /wp-content/uploads/.*\.php$`) {
		t.Fatal("upload PHP block missing")
	}
	if !strings.Contains(cfg, "server_name shop.example.com;") {
		t.Fatal("server_name missing")
	}
	// The PHP snippet already sets try_files; a second one makes nginx refuse the config.
	if n := strings.Count(cfg, "try_files"); n != 1 {
		t.Fatalf("expected exactly one try_files directive in a location, got %d", n)
	}
}

func TestWriteFileAtomicReplaces(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "site.conf")
	if err := writeFileAtomic(p, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(p, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "two" {
		t.Fatalf("got %q", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %d entries", len(entries))
	}
}
