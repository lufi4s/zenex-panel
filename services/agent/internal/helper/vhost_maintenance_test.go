package helper

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMaintenanceConfigAnswers503WithoutPHP(t *testing.T) {
	cfg := vhostConfig("zx_shop", "shop.example.com", "/var/www/zx_shop/htdocs", "/var/log/caddy", true)
	for _, want := range []string{
		"respond `",
		"Under maintenance - back soon",
		" 503\n",
		"header Retry-After 3600",
		"@uploads_php path_regexp uploads",
		"@dotfiles path_regexp dotfiles",
		"output file /var/log/caddy/zx-zx_shop.log",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("maintenance config missing %q", want)
		}
	}
	if !strings.Contains(cfg, "respond") || strings.Contains(cfg, "php_fastcgi") {
		t.Fatal("maintenance config must answer with respond and never run PHP-FPM")
	}
}

func TestNormalConfigStillProxiesToPHP(t *testing.T) {
	cfg := vhostConfig("zx_shop", "shop.example.com", "/var/www/zx_shop/htdocs", "/var/log/caddy", false)
	if !strings.Contains(cfg, "php_fastcgi unix//run/php/zx-zx_shop.sock") {
		t.Fatal("normal config does not proxy to PHP-FPM")
	}
	if strings.Contains(cfg, "Retry-After") || strings.Contains(cfg, " 503") {
		t.Fatal("normal config contains maintenance response")
	}
}

func TestVhostWriteMaintenanceFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses symbolic links, POSIX only")
	}
	f := &scriptExec{}
	o, _ := testOps(t, f)
	for _, dir := range []string{o.Paths.CaddyAvailable, o.Paths.CaddyEnabled} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, bad := range []string{"yes", "2", "true"} {
		args := map[string]string{"user": "zx_shop", "domain": "shop.example.com", "maintenance": bad}
		if _, err := o.Do(context.Background(), "vhost.write", args); err == nil {
			t.Fatalf("maintenance flag %q accepted", bad)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("caddy ran for invalid flag: %v", f.calls)
	}

	args := map[string]string{"user": "zx_shop", "domain": "shop.example.com", "maintenance": "1"}
	if _, err := o.Do(context.Background(), "vhost.write", args); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(o.Paths.CaddyAvailable, "zx-zx_shop.caddy"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "respond `") || strings.Contains(string(written), "php_fastcgi") {
		t.Fatal("written site file is not in maintenance mode")
	}
	if len(f.argv) < 2 || f.argv[0][0] != "/usr/bin/caddy" || f.argv[0][1] != "validate" {
		t.Fatalf("caddy validation did not run before reload: %v", f.argv)
	}
}
