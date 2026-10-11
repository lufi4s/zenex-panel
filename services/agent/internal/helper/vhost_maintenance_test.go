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
	cfg := vhostConfig("zx_shop", "shop.example.com", "/var/www/zx_shop/htdocs", "/var/log/caddy", true, false)
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
	cfg := vhostConfig("zx_shop", "shop.example.com", "/var/www/zx_shop/htdocs", "/var/log/caddy", false, false)
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

func TestPageCacheConfigServesSavedPagesOnlyToAnonymousPlainRequests(t *testing.T) {
	on := vhostConfig("zx_shop", "shop.example.com", "/var/www/zx_shop/htdocs", "/var/log/caddy", false, true)
	for _, want := range []string{
		"@zxcached {",
		"method GET",
		"expression {http.request.uri.query} == ''",
		"not path *..*",
		"not header_regexp Cookie (wordpress_logged_in|wordpress_sec|comment_author|wp-postpass|woocommerce_items_in_cart|woocommerce_cart_hash)",
		"try_files /wp-content/cache/zenex/{host}{path}index.html",
		"rewrite * {http.matchers.file.relative}",
	} {
		if !strings.Contains(on, want) {
			t.Errorf("cache config missing %q", want)
		}
	}
	// The saved page is tried before PHP, and phpMyAdmin is never answered from the cache.
	if strings.Index(on, "@zxcached") > strings.Index(on, "php_fastcgi unix//run/php/zx-zx_shop.sock\n        file_server") ||
		strings.Index(on, "handle_path /zenex-dbadmin*") > strings.Index(on, "@zxcached") {
		t.Error("handlers are in the wrong order")
	}
	off := vhostConfig("zx_shop", "shop.example.com", "/var/www/zx_shop/htdocs", "/var/log/caddy", false, false)
	if strings.Contains(off, "zxcached") {
		t.Error("cache config present while the cache is off")
	}
	maint := vhostConfig("zx_shop", "shop.example.com", "/var/www/zx_shop/htdocs", "/var/log/caddy", true, true)
	if strings.Contains(maint, "zxcached") {
		t.Error("a site in maintenance must never answer from the cache")
	}
}

func TestPageCacheDisableRemovesThePluginAndEverySavedPage(t *testing.T) {
	ex := &mailExec{}
	o, _ := testOps(t, ex)
	doc := o.docRoot("zx_shop")
	saved := filepath.Join(doc, "wp-content", "cache", "zenex", "a.example.com", "about")
	plugin := filepath.Join(doc, "wp-content", "mu-plugins", "zenex-page-cache.php")
	for _, dir := range []string{saved, filepath.Dir(plugin)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(saved, "index.html"), plugin} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := o.Do(context.Background(), "cache.clear", map[string]string{"user": "zx_shop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plugin); err != nil {
		t.Fatal("clearing the saved pages must keep the plugin")
	}
	if _, err := os.Stat(filepath.Join(doc, "wp-content", "cache", "zenex")); !os.IsNotExist(err) {
		t.Error("saved pages left behind after clear")
	}
	if _, err := o.Do(context.Background(), "cache.disable", map[string]string{"user": "zx_shop"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plugin); !os.IsNotExist(err) {
		t.Error("plugin left behind after disable")
	}
	if _, err := o.Do(context.Background(), "cache.clear", map[string]string{"user": "bad user"}); err == nil {
		t.Error("an invalid account name was accepted")
	}
}

func TestPageCachePluginNeverSavesPersonalOrDynamicPages(t *testing.T) {
	for _, want := range []string{
		"is_user_logged_in()", "!empty($_SERVER['QUERY_STRING'])", "'REQUEST_METHOD'", "post_password_required()",
		"is_cart()", "wordpress_logged_in", "Set-Cookie:", "substr($path, -1) !== '/'", "http_response_code() !== 200",
		"'save_post'", "'switch_theme'",
	} {
		if !strings.Contains(pageCachePluginSource, want) {
			t.Errorf("plugin is missing %q", want)
		}
	}
}

func TestPageCachePluginIsNotFlaggedByTheMalwareScanner(t *testing.T) {
	if f := scanContent("wp-content/mu-plugins/"+pageCachePlugin, []byte(pageCachePluginSource)); len(f) != 0 {
		t.Fatalf("the scanner would quarantine the page cache plugin: %+v", f)
	}
}
