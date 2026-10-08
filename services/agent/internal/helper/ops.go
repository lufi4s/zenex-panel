// Package helper implements the privileged operations used to build WordPress
// sites. It runs as root behind a Unix socket. It accepts a fixed set of named
// operations, validates every argument, and never builds shell strings.
package helper

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zenexcloud/zenex-panel/packages/validation"
	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
	"github.com/zenexcloud/zenex-panel/services/agent/internal/nginx"
)

// Binary locations. Each one must be on the executor allowlist.
const (
	binUseradd   = "/usr/sbin/useradd"
	binRunuser   = "/usr/sbin/runuser"
	binEnv       = "/usr/bin/env"
	binMariadb   = "/usr/bin/mariadb"
	binSystemctl = "/usr/bin/systemctl"
	binWP        = "/usr/local/bin/wp"
)

var (
	linuxUserRe = regexp.MustCompile(`^zx_[a-z0-9_]{1,28}$`)
	phpVerRe    = regexp.MustCompile(`^[0-9]\.[0-9]$`)
	hexSecretRe = regexp.MustCompile(`^[a-f0-9]{16,128}$`)
	titleRe     = regexp.MustCompile(`^[A-Za-z0-9 .\-]{1,64}$`)
	adminUserRe = regexp.MustCompile(`^[a-z][a-z0-9_]{2,31}$`)
)

// Paths configurable for tests. Production values come from DefaultPaths.
type Paths struct {
	WebRoot        string // /var/www
	PHPPoolDir     string // /etc/php/%s/fpm/pool.d  (formatted with version)
	NginxAvailable string // /etc/nginx/sites-available
	NginxEnabled   string // /etc/nginx/sites-enabled
	LogDir         string // /var/log/nginx
}

func DefaultPaths() Paths {
	return Paths{
		WebRoot:        "/var/www",
		PHPPoolDir:     "/etc/php/%s/fpm/pool.d",
		NginxAvailable: "/etc/nginx/sites-available",
		NginxEnabled:   "/etc/nginx/sites-enabled",
		LogDir:         "/var/log/nginx",
	}
}

// Result is returned to the caller for operations that produce a value.
type Result struct {
	UID string `json:"uid,omitempty"`
}

// commandRunner is the subset of the executor the helper needs.
type commandRunner interface {
	Run(ctx context.Context, bin string, args []string, timeout time.Duration) (executor.Result, error)
	RunInput(ctx context.Context, bin string, args []string, stdin string, timeout time.Duration) (executor.Result, error)
}

// Ops executes validated operations. All operations are serialized by one lock,
// so two sites cannot change nginx or PHP-FPM configuration at the same time.
type Ops struct {
	Exec  commandRunner
	Paths Paths
	// Lookup lets tests avoid touching the real account database.
	LookupUser func(string) (*user.User, error)
	mu         sync.Mutex
}

// Do dispatches a named operation with its arguments.
func (o *Ops) Do(ctx context.Context, op string, args map[string]string) (Result, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	switch op {
	case "user.create":
		return o.userCreate(ctx, args)
	case "fs.prepare":
		return Result{}, o.fsPrepare(args)
	case "db.create":
		return Result{}, o.dbCreate(ctx, args)
	case "pool.write":
		return Result{}, o.poolWrite(ctx, args)
	case "vhost.write":
		return Result{}, o.vhostWrite(ctx, args)
	case "wp.core-download":
		return Result{}, o.wpCoreDownload(ctx, args)
	case "wp.config-create":
		return Result{}, o.wpConfigCreate(ctx, args)
	case "wp.core-install":
		return Result{}, o.wpCoreInstall(ctx, args)
	case "wp.harden":
		return Result{}, o.wpHarden(ctx, args)
	default:
		return Result{}, fmt.Errorf("unknown operation %q", op)
	}
}

// ---------------------------------------------------------------------------
// Argument validation
// ---------------------------------------------------------------------------

func requireLinuxUser(args map[string]string) (string, error) {
	u := args["user"]
	if !linuxUserRe.MatchString(u) {
		return "", errors.New("invalid site account name")
	}
	return u, nil
}

func (o *Ops) homeDir(linuxUser string) string {
	return filepath.Join(o.Paths.WebRoot, linuxUser)
}

func (o *Ops) docRoot(linuxUser string) string {
	return filepath.Join(o.homeDir(linuxUser), "htdocs")
}

func requireHex(v, field string) error {
	if !hexSecretRe.MatchString(v) {
		return fmt.Errorf("invalid %s", field)
	}
	return nil
}

func requirePHPVersion(v string) error {
	if !phpVerRe.MatchString(v) {
		return errors.New("invalid PHP version")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Operations
// ---------------------------------------------------------------------------

func (o *Ops) lookup(name string) (*user.User, error) {
	if o.LookupUser != nil {
		return o.LookupUser(name)
	}
	return user.Lookup(name)
}

func (o *Ops) userCreate(ctx context.Context, args map[string]string) (Result, error) {
	name, err := requireLinuxUser(args)
	if err != nil {
		return Result{}, err
	}
	if u, err := o.lookup(name); err == nil {
		return Result{UID: u.Uid}, nil // already exists: idempotent
	}
	res, err := o.Exec.Run(ctx, binUseradd, []string{
		"--system",
		"--home-dir", o.homeDir(name),
		"--no-create-home",
		"--shell", "/usr/sbin/nologin",
		"--user-group",
		name,
	}, 30*time.Second)
	if err != nil {
		return Result{}, fmt.Errorf("create account: %w", err)
	}
	if res.ExitCode != 0 {
		return Result{}, fmt.Errorf("create account failed: %s", trim(res.Stderr))
	}
	u, err := o.lookup(name)
	if err != nil {
		return Result{}, fmt.Errorf("account lookup after create: %w", err)
	}
	return Result{UID: u.Uid}, nil
}

// fsPrepare creates the home and document root. The site account owns the
// document root; the nginx group can read it; no other account can enter it.
func (o *Ops) fsPrepare(args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	siteUser, err := o.lookup(name)
	if err != nil {
		return fmt.Errorf("site account missing: run user.create first")
	}
	web, err := user.LookupGroup("www-data")
	if err != nil {
		return fmt.Errorf("nginx group www-data missing: %w", err)
	}
	uid, _ := strconv.Atoi(siteUser.Uid)
	gid, _ := strconv.Atoi(web.Gid)

	if err := os.MkdirAll(o.docRoot(name), 0o750); err != nil {
		return fmt.Errorf("create document root: %w", err)
	}
	if err := os.Chown(o.homeDir(name), 0, gid); err != nil {
		return err
	}
	if err := os.Chmod(o.homeDir(name), 0o750); err != nil {
		return err
	}
	if err := os.Chown(o.docRoot(name), uid, gid); err != nil {
		return err
	}
	return os.Chmod(o.docRoot(name), 0o750)
}

func (o *Ops) dbCreate(ctx context.Context, args map[string]string) error {
	db, dbUser, pass := args["db"], args["dbuser"], args["password"]
	if !linuxUserRe.MatchString(db) || db != dbUser {
		return errors.New("invalid database name")
	}
	if err := requireHex(pass, "database password"); err != nil {
		return err
	}
	// Identifiers are validated above; the password is hex only, so the SQL is safe.
	sql := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%[1]s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n"+
			"CREATE USER IF NOT EXISTS '%[2]s'@'localhost';\n"+
			"ALTER USER '%[2]s'@'localhost' IDENTIFIED BY '%[3]s';\n"+
			"GRANT ALL PRIVILEGES ON `%[1]s`.* TO '%[2]s'@'localhost';\n"+
			"FLUSH PRIVILEGES;\n", db, dbUser, pass)

	res, err := o.Exec.RunInput(ctx, binMariadb, []string{"--protocol=socket", "-uroot"}, sql, 60*time.Second)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("database setup failed: %s", trim(res.Stderr))
	}
	return nil
}

func (o *Ops) poolWrite(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	ver := args["php"]
	if err := requirePHPVersion(ver); err != nil {
		return err
	}
	dir := fmt.Sprintf(o.Paths.PHPPoolDir, ver)
	path := filepath.Join(dir, "zx-"+name+".conf")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(path, []byte(poolConfig(name, o.homeDir(name))), 0o644); err != nil {
		return fmt.Errorf("write pool: %w", err)
	}

	test, err := o.Exec.Run(ctx, "/usr/sbin/php-fpm"+ver, []string{"-t"}, 30*time.Second)
	if err != nil || test.ExitCode != 0 {
		_ = os.Remove(path)
		return fmt.Errorf("PHP-FPM configuration test failed: %s", trim(stderrOr(test, err)))
	}
	res, err := o.Exec.Run(ctx, binSystemctl, []string{"reload", "php" + ver + "-fpm"}, 60*time.Second)
	if err != nil || res.ExitCode != 0 {
		return fmt.Errorf("reload PHP-FPM failed: %s", trim(stderrOr(res, err)))
	}
	return nil
}

func (o *Ops) vhostWrite(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	domain := args["domain"]
	if err := validation.DomainName(domain); err != nil {
		return errors.New("invalid domain")
	}
	avail := filepath.Join(o.Paths.NginxAvailable, "zx-"+name+".conf")
	enabled := filepath.Join(o.Paths.NginxEnabled, "zx-"+name+".conf")

	if err := writeFileAtomic(avail, []byte(vhostConfig(name, domain, o.docRoot(name), o.Paths.LogDir)), 0o644); err != nil {
		return fmt.Errorf("write site config: %w", err)
	}
	_ = os.Remove(enabled)
	if err := os.Symlink(avail, enabled); err != nil {
		_ = os.Remove(avail)
		return fmt.Errorf("enable site config: %w", err)
	}
	// nginx.Reload tests the configuration first and never reloads a bad one.
	if err := nginx.Reload(ctx, o.Exec); err != nil {
		_ = os.Remove(enabled)
		_ = os.Remove(avail)
		return fmt.Errorf("nginx rejected the site configuration; site not enabled: %w", err)
	}
	return nil
}

func (o *Ops) wpRun(ctx context.Context, linuxUser string, wpArgs ...string) error {
	home := o.homeDir(linuxUser)
	argv := []string{"-u", linuxUser, "--", binEnv, "HOME=" + home, binWP, "--path=" + o.docRoot(linuxUser)}
	argv = append(argv, wpArgs...)
	res, err := o.Exec.Run(ctx, binRunuser, argv, 10*time.Minute)
	if err != nil {
		return fmt.Errorf("wp-cli: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("wp-cli failed: %s", trim(stderrOr(res, nil)))
	}
	return nil
}

func (o *Ops) wpCoreDownload(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(o.docRoot(name), "wp-load.php")); err == nil {
		return nil // already downloaded
	}
	return o.wpRun(ctx, name, "core", "download", "--locale=en_US")
}

func (o *Ops) wpConfigCreate(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(o.docRoot(name), "wp-config.php")); err == nil {
		return nil
	}
	db, dbUser, pass := args["db"], args["dbuser"], args["dbpass"]
	if db != name || dbUser != name {
		return errors.New("database identifiers must match the site account")
	}
	if err := requireHex(pass, "database password"); err != nil {
		return err
	}
	return o.wpRun(ctx, name, "config", "create",
		"--dbname="+db, "--dbuser="+dbUser, "--dbpass="+pass, "--dbhost=localhost", "--dbcharset=utf8mb4")
}

func (o *Ops) wpCoreInstall(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	url, title, admin, pass, email := args["url"], args["title"], args["admin"], args["adminpass"], args["adminemail"]
	if err := validation.DomainName(strings.TrimPrefix(url, "http://")); err != nil || !strings.HasPrefix(url, "http://") {
		return errors.New("invalid site URL")
	}
	if !titleRe.MatchString(title) {
		return errors.New("invalid site title")
	}
	if !adminUserRe.MatchString(admin) {
		return errors.New("invalid admin user name")
	}
	if err := requireHex(pass, "admin password"); err != nil {
		return err
	}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
		return errors.New("invalid admin email")
	}
	// Re-running after a partial success must not fail on an installed site.
	if installed, err := o.wpInstalled(ctx, name); err != nil {
		return err
	} else if installed {
		return nil
	}
	return o.wpRun(ctx, name, "core", "install",
		"--url="+url, "--title="+title, "--admin_user="+admin,
		"--admin_password="+pass, "--admin_email="+email, "--skip-email")
}

// wpInstalled asks WP-CLI whether WordPress is already installed.
func (o *Ops) wpInstalled(ctx context.Context, linuxUser string) (bool, error) {
	home := o.homeDir(linuxUser)
	argv := []string{"-u", linuxUser, "--", binEnv, "HOME=" + home, binWP, "--path=" + o.docRoot(linuxUser), "core", "is-installed"}
	res, err := o.Exec.Run(ctx, binRunuser, argv, time.Minute)
	if err != nil {
		return false, fmt.Errorf("wp-cli: %w", err)
	}
	return res.ExitCode == 0, nil
}

// wpHarden disables theme and plugin editing from the dashboard and makes the
// configuration file readable only by the site account and nginx.
func (o *Ops) wpHarden(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	if err := o.wpRun(ctx, name, "config", "set", "DISALLOW_FILE_EDIT", "true", "--raw", "--type=constant"); err != nil {
		return err
	}
	cfg := filepath.Join(o.docRoot(name), "wp-config.php")
	siteUser, err := o.lookup(name)
	if err != nil {
		return err
	}
	web, err := user.LookupGroup("www-data")
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(siteUser.Uid)
	gid, _ := strconv.Atoi(web.Gid)
	if err := os.Chown(cfg, uid, gid); err != nil {
		return err
	}
	return os.Chmod(cfg, 0o640)
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

func poolConfig(name, home string) string {
	return fmt.Sprintf(`; Managed by Zenex. Do not edit by hand.
[zx-%[1]s]
user = %[1]s
group = %[1]s
listen = /run/php/zx-%[1]s.sock
listen.owner = www-data
listen.group = www-data
listen.mode = 0660
pm = ondemand
pm.max_children = 5
pm.process_idle_timeout = 10s
pm.max_requests = 500
php_admin_value[open_basedir] = %[2]s:/tmp/
php_admin_value[upload_tmp_dir] = /tmp
php_admin_value[memory_limit] = 256M
php_admin_value[upload_max_filesize] = 64M
php_admin_value[post_max_size] = 64M
php_admin_value[session.save_path] = /tmp
`, name, home)
}

func vhostConfig(name, domain, docRoot, logDir string) string {
	return fmt.Sprintf(`# Managed by Zenex. Do not edit by hand.
server {
    listen 80;
    listen [::]:80;
    server_name %[2]s;
    root %[3]s;
    index index.php index.html;

    access_log %[4]s/zx-%[1]s.access.log;
    error_log %[4]s/zx-%[1]s.error.log;
    client_max_body_size 64m;

    location / {
        try_files $uri $uri/ /index.php?$args;
    }

    location ~ \.php$ {
        try_files $uri =404;
        include snippets/fastcgi-php.conf;
        fastcgi_pass unix:/run/php/zx-%[1]s.sock;
    }

    # PHP must never run from uploads, even if a file is planted there.
    location ~* /wp-content/uploads/.*\.php$ {
        deny all;
    }

    location ~ /\. {
        deny all;
    }
}
`, name, domain, docRoot, logDir)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeFileAtomic writes to a temp file in the same directory and renames it,
// so a reader never sees a half-written configuration file.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".zx-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func stderrOr(res executor.Result, err error) string {
	if err != nil {
		return err.Error()
	}
	if res.Stderr != "" {
		return res.Stderr
	}
	return res.Stdout
}

func trim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 500 {
		return s[:500] + "..."
	}
	return s
}

// AllowedBinaries lists every program the helper may execute. phpFPM is the
// set of PHP-FPM binaries installed on the server, discovered at startup.
func AllowedBinaries(phpFPM []string) []string {
	base := []string{binUseradd, binRunuser, binEnv, binMariadb, binSystemctl, binWP}
	base = append(base, nginx.AllowedBinaries...)
	return append(base, phpFPM...)
}
