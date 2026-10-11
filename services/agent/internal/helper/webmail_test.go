package helper

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// webmailTestOps builds an Ops with a temporary webmail program, PHP versions 8.2 and 8.3,
// and temporary PHP pool and Caddy folders.
func webmailTestOps(t *testing.T, ex commandRunner, installed bool) (*Ops, string) {
	t.Helper()
	o, root := testOps(t, ex)
	o.Paths.WebmailDir = filepath.Join(root, "rc")
	if installed {
		if err := os.MkdirAll(filepath.Join(o.Paths.WebmailDir, "public_html"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(o.Paths.WebmailDir, "public_html", "index.php"), []byte("<?php"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	phpDir := filepath.Join(root, "php")
	if err := os.MkdirAll(phpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"8.2", "8.3"} {
		if err := os.WriteFile(filepath.Join(phpDir, "php-fpm"+v), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	o.Paths.PHPFPMGlob = filepath.Join(phpDir, "php-fpm[0-9]*.[0-9]*")
	return o, root
}

// skipWithoutSymlinks skips a test that enables a Caddy site, which is a symbolic link.
func skipWithoutSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses symbolic links, POSIX only")
	}
}

func TestWebmailSetupPublishesPoolAndSiteAfterChecks(t *testing.T) {
	skipWithoutSymlinks(t)
	ex := &mailExec{}
	o, _ := webmailTestOps(t, ex, true)
	res, err := o.Do(context.Background(), "webmail.setup", map[string]string{"host": "mail.ozima.cloud"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "https://mail.ozima.cloud/" {
		t.Fatalf("url = %q", res.Output)
	}

	pool, err := os.ReadFile(filepath.Join(o.Paths.PHPPoolDir[:strings.Index(o.Paths.PHPPoolDir, "%s")]+"8.3", "zenex-webmail.conf"))
	if err != nil {
		t.Fatalf("pool not written for the newest PHP: %v", err)
	}
	for _, want := range []string{"[zenex-webmail]", "user = zenex-webmail", "listen = /run/php/zenex-webmail.sock", "open_basedir]", "disable_functions]"} {
		if !strings.Contains(string(pool), want) {
			t.Errorf("pool missing %q", want)
		}
	}
	site, err := os.ReadFile(filepath.Join(o.Paths.CaddyAvailable, "zenex-webmail.caddy"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"mail.ozima.cloud {", "/public_html", "php_fastcgi unix//run/php/zenex-webmail.sock", "@blocked path /installer.php", "X-Frame-Options SAMEORIGIN"} {
		if !strings.Contains(string(site), want) {
			t.Errorf("site missing %q", want)
		}
	}
	if target, err := os.Readlink(filepath.Join(o.Paths.CaddyEnabled, "zenex-webmail.caddy")); err != nil || !strings.HasSuffix(target, "zenex-webmail.caddy") {
		t.Fatalf("site not enabled: %v %q", err, target)
	}

	// Every check runs before its reload, PHP first, then the web server.
	order := []string{"php-fpm8.3 -t", "reload php8.3-fpm", "/usr/bin/caddy validate", "reload caddy"}
	last := -1
	for _, want := range order {
		found := -1
		for i, c := range ex.calls {
			if strings.Contains(c, want) && i > last {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("call %q missing or out of order in %v", want, ex.calls)
		}
		last = found
	}
}

func TestWebmailSetupAsksForAnUpdateWhenWebmailIsNotInstalled(t *testing.T) {
	ex := &mailExec{}
	o, _ := webmailTestOps(t, ex, false)
	_, err := o.Do(context.Background(), "webmail.setup", map[string]string{"host": "mail.ozima.cloud"})
	if err == nil || !strings.Contains(err.Error(), "panel update") {
		t.Fatalf("err = %v, want a message to run the panel update", err)
	}
	if len(ex.calls) != 0 {
		t.Fatalf("programs ran: %v", ex.calls)
	}
}

func TestWebmailSetupRejectsBadHostsBeforeRunningAnything(t *testing.T) {
	ex := &mailExec{}
	o, _ := webmailTestOps(t, ex, true)
	for _, bad := range []string{"localhost", "mail.example.com {", "Mail.Example.com", "a b.com", ""} {
		if _, err := o.Do(context.Background(), "webmail.setup", map[string]string{"host": bad}); err == nil {
			t.Errorf("host %q accepted", bad)
		}
	}
	if len(ex.calls) != 0 {
		t.Fatalf("programs ran for bad hosts: %v", ex.calls)
	}
}

func TestWebmailSetupLeavesNothingWhenThePHPCheckFails(t *testing.T) {
	ex := &mailExec{failBin: "/usr/sbin/php-fpm8.3"}
	o, _ := webmailTestOps(t, ex, true)
	if _, err := o.Do(context.Background(), "webmail.setup", map[string]string{"host": "mail.ozima.cloud"}); err == nil {
		t.Fatal("failed PHP check was accepted")
	}
	pool := filepath.Join(strings.Replace(o.Paths.PHPPoolDir, "%s", "8.3", 1), "zenex-webmail.conf")
	if _, err := os.Stat(pool); !os.IsNotExist(err) {
		t.Fatal("the pool file was left after a failed check")
	}
	for _, c := range ex.calls {
		if strings.Contains(c, "caddy") {
			t.Fatalf("the web server was touched after a failed PHP check: %v", ex.calls)
		}
	}
}

func TestWebmailSetupRestoresThePreviousSiteWhenCaddyRejectsTheNewOne(t *testing.T) {
	skipWithoutSymlinks(t)
	ex := &mailExec{failBin: "/usr/bin/caddy", failArg: "validate"}
	o, _ := webmailTestOps(t, ex, true)
	if err := os.MkdirAll(o.Paths.CaddyAvailable, 0o755); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(o.Paths.CaddyAvailable, "zenex-webmail.caddy")
	if err := os.WriteFile(site, []byte("old.example.com {\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Do(context.Background(), "webmail.setup", map[string]string{"host": "mail.ozima.cloud"}); err == nil {
		t.Fatal("rejected site was accepted")
	}
	got, _ := os.ReadFile(site)
	if string(got) != "old.example.com {\n}\n" {
		t.Fatalf("old site not restored: %q", got)
	}
	for _, c := range ex.calls {
		if strings.Contains(c, "reload caddy") {
			t.Fatalf("caddy reloaded after a failed validation: %v", ex.calls)
		}
	}
}

func TestWebmailSiteNeverServesTheInstallerOrProgramFolders(t *testing.T) {
	site := webmailSiteConfig("mail.example.com", "/usr/share/zenex-roundcube", "/var/log/caddy")
	for _, blocked := range []string{"/installer.php", "/installer/*", "/.*", "/config/*", "/logs/*", "/temp/*", "/SQL/*", "/bin/*", "/vendor/*"} {
		if !strings.Contains(site, blocked) {
			t.Errorf("site does not block %q", blocked)
		}
	}
	if !strings.Contains(site, "root * /usr/share/zenex-roundcube/public_html") {
		t.Error("the web root must be public_html, not the program folder")
	}
}
