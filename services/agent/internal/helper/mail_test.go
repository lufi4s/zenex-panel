package helper

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

// mailExec records every program run. failBin makes that program exit 1 with a message.
// It also plays openssl: genpkey and pkey write their output files, as the real tools do.
type mailExec struct {
	calls   []string
	failBin string
	failArg string
}

func (m *mailExec) Run(_ context.Context, bin string, args []string, _ time.Duration) (executor.Result, error) {
	m.calls = append(m.calls, bin+" "+strings.Join(args, " "))
	if bin == binOpenSSL && args[0] == "req" {
		// Like the real tool: write the key and the certificate to the paths it is given.
		for i, a := range args {
			if (a == "-keyout" || a == "-out") && i+1 < len(args) {
				if err := os.WriteFile(args[i+1], []byte("PEM "+a), 0o644); err != nil {
					return executor.Result{}, err
				}
			}
		}
		return executor.Result{}, nil
	}
	if bin == binOpenSSL {
		out := args[len(args)-1]
		if args[0] == "genpkey" {
			if err := os.WriteFile(out, []byte("PRIVATE KEY"), 0o644); err != nil {
				return executor.Result{}, err
			}
		}
		if args[0] == "pkey" {
			if err := os.WriteFile(out, []byte("-----BEGIN PUBLIC KEY-----\nQUJD\nREVG\n-----END PUBLIC KEY-----\n"), 0o644); err != nil {
				return executor.Result{}, err
			}
		}
		return executor.Result{}, nil
	}
	if bin == m.failBin && (m.failArg == "" || strings.Contains(strings.Join(args, " "), m.failArg)) {
		return executor.Result{ExitCode: 1, Stderr: "config error near line 4 password=hunter2-secret"}, nil
	}
	return executor.Result{}, nil
}

func (m *mailExec) RunInput(ctx context.Context, bin string, args []string, _ string, timeout time.Duration) (executor.Result, error) {
	return m.Run(ctx, bin, args, timeout)
}

func (m *mailExec) RunEnv(ctx context.Context, bin string, args []string, _ []string, timeout time.Duration) (executor.Result, error) {
	return m.Run(ctx, bin, args, timeout)
}

func (m *mailExec) RunToFile(ctx context.Context, bin string, args []string, _ []string, _ string, timeout time.Duration) (executor.Result, error) {
	return m.Run(ctx, bin, args, timeout)
}

func mailTestOps(t *testing.T, ex commandRunner) (*Ops, string) {
	t.Helper()
	root := t.TempDir()
	o, _ := testOps(t, ex)
	o.MailRoot = filepath.Join(root, "mail")
	o.Paths.DKIMDir = filepath.Join(root, "dkim")
	return o, root
}

func applyArgs(t *testing.T, files []mailFile) map[string]string {
	t.Helper()
	b, err := json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"files": string(b), "dbpass": "hunter2-secret"}
}

func TestMailApplyWritesChecksAndReloads(t *testing.T) {
	ex := &mailExec{}
	o, _ := mailTestOps(t, ex)
	files := []mailFile{{Path: "/etc/postfix/main.cf", Mode: 0o644, Content: "myhostname = mail.example.com\n"}}
	if _, err := o.Do(context.Background(), "mail.apply", applyArgs(t, files)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(o.MailRoot, "/etc/postfix/main.cf"))
	if err != nil || string(got) != files[0].Content {
		t.Fatalf("file not written: %v %q", err, got)
	}
	want := []string{
		binPostfix + " check",
		binDoveconf + " -n",
		binRspamadm + " configtest",
		binSystemctl + " enable --now postfix",
		binSystemctl + " reload postfix",
		binSystemctl + " enable --now dovecot",
		binSystemctl + " reload dovecot",
		binSystemctl + " enable --now rspamd",
		binSystemctl + " reload rspamd",
	}
	if strings.Join(ex.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(ex.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestMailApplyRollsBackWhenACheckFails(t *testing.T) {
	ex := &mailExec{failBin: binDoveconf}
	o, _ := mailTestOps(t, ex)
	target := filepath.Join(o.MailRoot, "/etc/postfix/main.cf")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []mailFile{{Path: "/etc/postfix/main.cf", Mode: 0o644, Content: "broken new config\n"}}
	_, err := o.Do(context.Background(), "mail.apply", applyArgs(t, files))
	if err == nil {
		t.Fatal("failed check was accepted")
	}
	if strings.Contains(err.Error(), "hunter2-secret") {
		t.Fatalf("database password leaked into the error: %v", err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "old config\n" {
		t.Fatalf("old config not restored: %q", got)
	}
	for _, c := range ex.calls {
		if strings.Contains(c, "reload") {
			t.Fatalf("services reloaded after a failed check: %v", ex.calls)
		}
	}
}

func TestMailApplyRemovesNewFilesWhenACheckFails(t *testing.T) {
	ex := &mailExec{failBin: binPostfix}
	o, _ := mailTestOps(t, ex)
	files := []mailFile{{Path: "/etc/postfix/zenex-domains.cf", Mode: 0o640, Content: "hosts = 127.0.0.1\n"}}
	if _, err := o.Do(context.Background(), "mail.apply", applyArgs(t, files)); err == nil {
		t.Fatal("failed check was accepted")
	}
	if _, err := os.Stat(filepath.Join(o.MailRoot, "/etc/postfix/zenex-domains.cf")); !os.IsNotExist(err) {
		t.Fatal("a new file was left behind after a failed check")
	}
}

func TestMailApplyOnlyWritesAllowlistedPaths(t *testing.T) {
	ex := &mailExec{}
	o, _ := mailTestOps(t, ex)
	bad := [][]mailFile{
		{{Path: "/etc/passwd", Mode: 0o644, Content: "x"}},
		{{Path: "/etc/postfix/main.cf/../../../root/.ssh/authorized_keys", Mode: 0o644, Content: "x"}},
		{{Path: "/etc/postfix/main.cf", Mode: 0o600, Content: "x"}},
		{{Path: "/etc/postfix/main.cf", Mode: 0o644, Content: ""}},
		{{Path: "/etc/postfix/main.cf", Mode: 0o644, Content: "a"}, {Path: "/etc/postfix/main.cf", Mode: 0o644, Content: "b"}},
	}
	for i, files := range bad {
		if _, err := o.Do(context.Background(), "mail.apply", applyArgs(t, files)); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	if len(ex.calls) != 0 {
		t.Fatalf("programs ran for rejected input: %v", ex.calls)
	}
}

func TestDKIMKeyCreatedOnceAndReturnsTheRecordValue(t *testing.T) {
	ex := &mailExec{}
	o, _ := mailTestOps(t, ex)
	first, err := o.Do(context.Background(), "mail.dkim-key", map[string]string{"domain": "ozima.cloud"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Output != "p=QUJDREVG" {
		t.Fatalf("DKIM value = %q", first.Output)
	}
	keyInfo, err := os.Stat(filepath.Join(o.Paths.DKIMDir, "ozima.cloud", "default.key"))
	if err != nil {
		t.Fatalf("private key missing: %v", err)
	}
	// Windows does not keep POSIX permission bits, so the mode is checked on Linux only.
	if runtime.GOOS != "windows" && keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode = %o, want 0600", keyInfo.Mode().Perm())
	}
	second, err := o.Do(context.Background(), "mail.dkim-key", map[string]string{"domain": "ozima.cloud"})
	if err != nil || second.Output != first.Output {
		t.Fatalf("second call = %q, %v", second.Output, err)
	}
	generated := 0
	for _, c := range ex.calls {
		if strings.Contains(c, "genpkey") {
			generated++
		}
	}
	if generated != 1 {
		t.Fatalf("key generated %d times, want once", generated)
	}
}

func TestDKIMKeyRejectsBadDomains(t *testing.T) {
	ex := &mailExec{}
	o, _ := mailTestOps(t, ex)
	for _, d := range []string{"../etc", "localhost", "a b.com", ""} {
		if _, err := o.Do(context.Background(), "mail.dkim-key", map[string]string{"domain": d}); err == nil {
			t.Errorf("domain %q accepted", d)
		}
	}
	if len(ex.calls) != 0 {
		t.Fatalf("openssl ran for bad domains: %v", ex.calls)
	}
}

func TestMailAllowlistMatchesTheAPIPaths(t *testing.T) {
	for _, path := range []string{
		"/etc/postfix/main.cf", "/etc/postfix/master.cf", "/etc/postfix/zenex-domains.cf",
		"/etc/postfix/zenex-mailboxes.cf", "/etc/dovecot/conf.d/zenex-sql.conf.ext",
		"/etc/dovecot/conf.d/99-zenex.conf", "/etc/rspamd/local.d/dkim_signing.conf",
	} {
		if _, ok := mailFileAllowlist[path]; !ok {
			t.Errorf("%s missing from the helper allowlist", path)
		}
	}
}

func countCalls(calls []string, prefix string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func TestMailPrepareCreatesACertificateOnceAndOpensThePorts(t *testing.T) {
	ex := &mailExec{}
	o, _ := mailTestOps(t, ex)
	if _, err := o.Do(context.Background(), "mail.prepare", map[string]string{"host": "mail.example.com"}); err != nil {
		t.Fatal(err)
	}
	if got := countCalls(ex.calls, binOpenSSL+" req"); got != 1 {
		t.Fatalf("certificate created %d times, want once", got)
	}
	for _, port := range []string{"25/tcp", "587/tcp", "993/tcp"} {
		if countCalls(ex.calls, binUfw+" allow "+port) != 1 {
			t.Errorf("port %s was not opened: %v", port, ex.calls)
		}
	}
	marker, err := os.ReadFile(filepath.Join(o.MailRoot, mailTLSDir, "host"))
	if err != nil || strings.TrimSpace(string(marker)) != "mail.example.com" {
		t.Fatalf("host marker = %q, %v", marker, err)
	}

	// Same host again: the existing certificate (maybe a real one) is kept.
	if _, err := o.Do(context.Background(), "mail.prepare", map[string]string{"host": "mail.example.com"}); err != nil {
		t.Fatal(err)
	}
	if got := countCalls(ex.calls, binOpenSSL+" req"); got != 1 {
		t.Fatalf("certificate replaced for the same host (%d creations)", got)
	}
	// A different host gets its own certificate.
	if _, err := o.Do(context.Background(), "mail.prepare", map[string]string{"host": "mx.example.org"}); err != nil {
		t.Fatal(err)
	}
	if got := countCalls(ex.calls, binOpenSSL+" req"); got != 2 {
		t.Fatalf("no new certificate for a new host (%d creations)", got)
	}
}

func TestMailPrepareRejectsBadHostsBeforeRunningAnything(t *testing.T) {
	ex := &mailExec{}
	o, _ := mailTestOps(t, ex)
	for _, bad := range []string{"localhost", "mail.example.com; reboot", "Mail.Example.com", "", "a b.com"} {
		if _, err := o.Do(context.Background(), "mail.prepare", map[string]string{"host": bad}); err == nil {
			t.Errorf("host %q accepted", bad)
		}
	}
	if len(ex.calls) != 0 {
		t.Fatalf("programs ran for rejected hosts: %v", ex.calls)
	}
}

func TestMailPrepareStopsWhenAPortCannotBeOpened(t *testing.T) {
	ex := &mailExec{failBin: binUfw}
	o, _ := mailTestOps(t, ex)
	if _, err := o.Do(context.Background(), "mail.prepare", map[string]string{"host": "mail.example.com"}); err == nil {
		t.Fatal("firewall failure was ignored")
	}
}
