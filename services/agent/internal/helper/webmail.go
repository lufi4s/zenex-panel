package helper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/caddy"
)

// Webmail (Roundcube). The installer puts the program in place with its own database and its
// own unprivileged account. This operation publishes it: a PHP-FPM pool that runs as that
// account and a Caddy site under the mail host name, so Caddy fetches the HTTPS certificate.
// Both files are checked before anything reloads and are put back if a check fails.

const (
	webmailPoolName    = "zenex-webmail"
	webmailAccount     = "zenex-webmail"
	webmailSiteFile    = "zenex-webmail.caddy"
	defaultWebmailDir  = "/usr/share/zenex-roundcube"
	webmailStateDir    = "/var/lib/zenex-roundcube"
	webmailLogDir      = "/var/log/zenex-roundcube"
	webmailPHPTestTime = 30 * time.Second
	webmailReloadTime  = 60 * time.Second
)

// webmailPoolConfig is the PHP-FPM pool: its own account, a socket for Caddy, and a PHP that can
// only see the webmail program, its state folder, its log folder and /tmp.
func webmailPoolConfig(root string) string {
	return fmt.Sprintf(`; Managed by Zenex. Do not edit by hand.
[%[1]s]
user = %[2]s
group = %[2]s
listen = /run/php/%[1]s.sock
listen.owner = www-data
listen.group = www-data
listen.mode = 0660
pm = ondemand
pm.max_children = 8
pm.process_idle_timeout = 30s
pm.max_requests = 500
php_admin_value[open_basedir] = %[3]s/:%[4]s/:%[5]s/:/tmp/
php_admin_value[upload_tmp_dir] = %[4]s/temp
php_admin_value[memory_limit] = 256M
php_admin_value[upload_max_filesize] = 32M
php_admin_value[post_max_size] = 40M
php_admin_value[disable_functions] = exec,passthru,shell_exec,system,proc_open,popen
php_admin_flag[expose_php] = off
`, webmailPoolName, webmailAccount, root, webmailStateDir, webmailLogDir)
}

// webmailSiteConfig is the Caddy site for the mail host. The installer page, hidden files and
// the program's own folders are never served; only public_html is the web root.
func webmailSiteConfig(host, root, logDir string) string {
	return fmt.Sprintf(`# Managed by Zenex. Do not edit by hand.
%[1]s {
    root * %[2]s/public_html
    encode zstd gzip

    log {
        output file %[3]s/zenex-webmail.log {
            roll_size 10mb
            roll_keep 3
        }
        format console
    }

    header {
        Strict-Transport-Security "max-age=31536000"
        X-Content-Type-Options nosniff
        X-Frame-Options SAMEORIGIN
        Referrer-Policy no-referrer
        -Server
    }

    @blocked path /installer.php /installer/* /.* /config/* /logs/* /temp/* /SQL/* /bin/* /vendor/*
    handle @blocked {
        respond 404
    }

    handle {
        php_fastcgi unix//run/php/%[4]s.sock
        file_server
    }
}
`, host, root, logDir, webmailPoolName)
}

func (o *Ops) webmailRoot() string {
	if o.Paths.WebmailDir != "" {
		return o.Paths.WebmailDir
	}
	return defaultWebmailDir
}

// webmailSetup publishes the webmail under host.
func (o *Ops) webmailSetup(ctx context.Context, args map[string]string) (Result, error) {
	host := args["host"]
	if !mailHostRe.MatchString(host) {
		return Result{}, errors.New("the mail host must be a full host name such as mail.example.com")
	}
	root := o.webmailRoot()
	if _, err := os.Stat(filepath.Join(root, "public_html", "index.php")); err != nil {
		return Result{}, errors.New("webmail is not installed on this server. Run the panel update, then try again")
	}
	versions := o.phpVersions()
	if len(versions) == 0 {
		return Result{}, errors.New("PHP-FPM is not installed on this server")
	}
	ver := versions[len(versions)-1]

	if err := o.writeWebmailPool(ctx, ver, root); err != nil {
		return Result{}, err
	}
	if err := o.writeWebmailSite(ctx, host, root); err != nil {
		return Result{}, err
	}
	return Result{Output: "https://" + host + "/"}, nil
}

// restoreFile puts a file back to what it was, or removes it when it did not exist.
func restoreFile(path string, previous []byte, existed bool, mode os.FileMode) {
	if existed {
		_ = writeFileAtomic(path, previous, mode)
		return
	}
	_ = os.Remove(path)
}

func (o *Ops) writeWebmailPool(ctx context.Context, ver, root string) error {
	dir := fmt.Sprintf(o.Paths.PHPPoolDir, ver)
	path := filepath.Join(dir, webmailPoolName+".conf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	previous, readErr := os.ReadFile(path)
	existed := readErr == nil
	if err := writeFileAtomic(path, []byte(webmailPoolConfig(root)), 0o644); err != nil {
		return fmt.Errorf("write the webmail PHP pool: %w", err)
	}
	test, err := o.Exec.Run(ctx, "/usr/sbin/php-fpm"+ver, []string{"-t"}, webmailPHPTestTime)
	if err != nil || test.ExitCode != 0 {
		restoreFile(path, previous, existed, 0o644)
		return fmt.Errorf("the PHP-FPM configuration test failed; nothing was changed: %s", trim(stderrOr(test, err)))
	}
	res, err := o.Exec.Run(ctx, binSystemctl, []string{"reload", "php" + ver + "-fpm"}, webmailReloadTime)
	if err != nil || res.ExitCode != 0 {
		return fmt.Errorf("reload PHP-FPM failed: %s", trim(stderrOr(res, err)))
	}
	return nil
}

func (o *Ops) writeWebmailSite(ctx context.Context, host, root string) error {
	avail := filepath.Join(o.Paths.CaddyAvailable, webmailSiteFile)
	enabled := filepath.Join(o.Paths.CaddyEnabled, webmailSiteFile)
	for _, dir := range []string{o.Paths.CaddyAvailable, o.Paths.CaddyEnabled} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	previous, readErr := os.ReadFile(avail)
	existed := readErr == nil
	if err := writeFileAtomic(avail, []byte(webmailSiteConfig(host, root, o.Paths.LogDir)), 0o644); err != nil {
		return fmt.Errorf("write the webmail site: %w", err)
	}
	_ = os.Remove(enabled)
	if err := os.Symlink(avail, enabled); err != nil {
		restoreFile(avail, previous, existed, 0o644)
		return fmt.Errorf("enable the webmail site: %w", err)
	}
	// caddy.Reload validates the whole configuration first and never reloads a bad one.
	if err := caddy.Reload(ctx, o.Exec); err != nil {
		restoreFile(avail, previous, existed, 0o644)
		if !existed {
			_ = os.Remove(enabled)
		}
		return fmt.Errorf("the web server rejected the webmail site; nothing was changed: %w", err)
	}
	return nil
}
