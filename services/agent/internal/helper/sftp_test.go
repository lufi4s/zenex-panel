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

// sftpExec records every program run and their stdin; failBin makes one program exit 1.
type sftpExec struct {
	argv    [][]string
	stdin   []string
	failBin string
}

func (s *sftpExec) Run(_ context.Context, bin string, args []string, _ time.Duration) (executor.Result, error) {
	return s.record(bin, args, "")
}

func (s *sftpExec) RunInput(_ context.Context, bin string, args []string, stdin string, _ time.Duration) (executor.Result, error) {
	return s.record(bin, args, stdin)
}

func (s *sftpExec) RunEnv(_ context.Context, bin string, args []string, _ []string, _ time.Duration) (executor.Result, error) {
	return s.record(bin, args, "")
}

func (s *sftpExec) RunToFile(_ context.Context, bin string, args []string, _ []string, _ string, _ time.Duration) (executor.Result, error) {
	return s.record(bin, args, "")
}

func (s *sftpExec) record(bin string, args []string, stdin string) (executor.Result, error) {
	s.argv = append(s.argv, append([]string{bin}, args...))
	s.stdin = append(s.stdin, stdin)
	if bin == s.failBin {
		return executor.Result{ExitCode: 1, Stderr: "boom"}, nil
	}
	return executor.Result{}, nil
}

const testSFTPPassword = "0123456789abcdef0123abcd"

func sftpTestOps(t *testing.T, ex commandRunner) (*Ops, string) {
	t.Helper()
	o, root := testOps(t, ex)
	o.Paths.SSHDropIn = filepath.Join(root, "sshd", "zenex-sftp.conf")
	if err := os.MkdirAll(filepath.Dir(o.Paths.SSHDropIn), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(o.homeDir("zx_shop"), 0o750); err != nil {
		t.Fatal(err)
	}
	return o, o.Paths.SSHDropIn
}

func TestSFTPEnableSetsPasswordOnStdinAndChecksSSH(t *testing.T) {
	f := &sftpExec{}
	o, dropIn := sftpTestOps(t, f)
	if _, err := o.Do(context.Background(), "sftp.enable", map[string]string{"user": "zx_shop", "password": testSFTPPassword}); err != nil {
		t.Fatal(err)
	}
	for _, call := range f.argv {
		if strings.Contains(strings.Join(call, " "), testSFTPPassword) {
			t.Fatalf("password appeared in argv: %v", call)
		}
	}
	var sawChpasswd, sawSSHTest bool
	for i, call := range f.argv {
		if call[0] == binChpasswd {
			sawChpasswd = f.stdin[i] == "zx_shop:"+testSFTPPassword+"\n"
		}
		if call[0] == binSSHD && len(call) > 1 && call[1] == "-t" {
			sawSSHTest = true
		}
	}
	if !sawChpasswd || !sawSSHTest {
		t.Fatalf("chpasswd with password on stdin = %v, sshd -t = %v; calls %v", sawChpasswd, sawSSHTest, f.argv)
	}
	written, err := os.ReadFile(dropIn)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Match Group zenex-sftp", "ChrootDirectory %h", "ForceCommand internal-sftp", "PermitTTY no"} {
		if !strings.Contains(string(written), want) {
			t.Errorf("drop-in missing %q", want)
		}
	}
}

func TestSFTPEnableRollsBackWhenSSHRejectsIt(t *testing.T) {
	f := &sftpExec{failBin: binSSHD}
	o, dropIn := sftpTestOps(t, f)
	if _, err := o.Do(context.Background(), "sftp.enable", map[string]string{"user": "zx_shop", "password": testSFTPPassword}); err == nil {
		t.Fatal("enable succeeded though sshd -t failed")
	}
	if _, err := os.Stat(dropIn); !os.IsNotExist(err) {
		t.Fatal("rejected SSH settings were left in place")
	}
	for _, call := range f.argv {
		if call[0] == binSystemctl {
			t.Fatal("ssh was reloaded after a failed test")
		}
	}
}

func TestSFTPDisableLocksPasswordAndLeavesGroup(t *testing.T) {
	f := &sftpExec{}
	o, _ := sftpTestOps(t, f)
	if _, err := o.Do(context.Background(), "sftp.disable", map[string]string{"user": "zx_shop"}); err != nil {
		t.Fatal(err)
	}
	var locked, removed bool
	for _, call := range f.argv {
		if call[0] == binPasswd && len(call) == 3 && call[1] == "-l" && call[2] == "zx_shop" {
			locked = true
		}
		if call[0] == binGpasswd && strings.Join(call[1:], " ") == "-d zx_shop "+sftpGroup {
			removed = true
		}
	}
	if !locked || !removed {
		t.Fatalf("locked = %v, removed from group = %v; calls %v", locked, removed, f.argv)
	}
}

func TestSFTPRejectsBadPasswordAndUser(t *testing.T) {
	f := &sftpExec{}
	o, _ := sftpTestOps(t, f)
	for _, args := range []map[string]string{
		{"user": "zx_shop", "password": "short"},
		{"user": "zx_shop", "password": "UPPERCASE0123456789ab"},
		{"user": "zx_shop", "password": "pass word 12345678"},
		{"user": "root", "password": testSFTPPassword},
	} {
		if _, err := o.Do(context.Background(), "sftp.enable", args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	if len(f.argv) != 0 {
		t.Fatalf("programs ran for invalid input: %v", f.argv)
	}
}

func TestWebsiteConfigOffersPhpMyAdminOnlyWhenNotInMaintenance(t *testing.T) {
	cfg := vhostConfig("zx_shop", "shop.example.com", "/var/www/zx_shop/htdocs", "/var/log/caddy", false, false)
	for _, want := range []string{"handle_path /zenex-dbadmin*", "root * " + PMADir, "php_fastcgi unix//run/php/zx-zx_shop.sock", "env SCRIPT_NAME /zenex-dbadmin/index.php"} {
		if !strings.Contains(cfg, want) {
			t.Errorf("site config missing %q", want)
		}
	}
	maint := vhostConfig("zx_shop", "shop.example.com", "/var/www/zx_shop/htdocs", "/var/log/caddy", true, false)
	if strings.Contains(maint, "zenex-dbadmin") {
		t.Fatal("maintenance mode still exposes phpMyAdmin")
	}
}

func TestPoolCanReadPhpMyAdmin(t *testing.T) {
	cfg := poolConfig("zx_shop", "/var/www/zx_shop")
	if !strings.Contains(cfg, "open_basedir] = /var/www/zx_shop:/tmp/:"+PMADir) {
		t.Fatalf("open_basedir does not include phpMyAdmin: %s", cfg)
	}
}
