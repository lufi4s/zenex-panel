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
	"github.com/zenexcloud/zenex-panel/services/agent/internal/caddy"
	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

// Binary locations. Each one must be on the executor allowlist.
const (
	binUseradd     = "/usr/sbin/useradd"
	binRunuser     = "/usr/sbin/runuser"
	binEnv         = "/usr/bin/env"
	binMariadb     = "/usr/bin/mariadb"
	binSystemctl   = "/usr/bin/systemctl"
	binWP          = "/usr/local/bin/wp"
	binGit         = "/usr/bin/git"
	binSystemdRun  = "/usr/bin/systemd-run"
	binBash        = "/usr/bin/bash"
	binMariadbDump = "/usr/bin/mariadb-dump"
	binTar         = "/usr/bin/tar"
	binSSHKeygen   = "/usr/bin/ssh-keygen"
	binSFTP        = "/usr/bin/sftp"
)

// Exit codes of env(1) when it cannot start the program (127) or the program is
// not executable (126). Any other code from a WP-CLI update is not a failure.
const (
	exitNotFound   = 127
	exitNotExec    = 126
	wpUpdateTime   = 15 * time.Minute
	backupDumpTime = 15 * time.Minute
	backupTarTime  = 15 * time.Minute
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
	CaddyAvailable string // /etc/caddy/zenex-available
	CaddyEnabled   string // /etc/caddy/zenex
	LogDir         string // /var/log/caddy
	PHPFPMGlob     string // PHP-FPM binaries installed on the server
	BackupDir      string // /var/backups/zenex
	BackupKeyDir   string // /etc/zenex/backup (SSH key pair, known_hosts, sftp batch files)
}

func DefaultPaths() Paths {
	return Paths{
		WebRoot:        "/var/www",
		PHPPoolDir:     "/etc/php/%s/fpm/pool.d",
		CaddyAvailable: "/etc/caddy/zenex-available",
		CaddyEnabled:   "/etc/caddy/zenex",
		LogDir:         "/var/log/caddy",
		PHPFPMGlob:     "/usr/sbin/php-fpm[0-9]*.[0-9]*",
		BackupDir:      "/var/backups/zenex",
		BackupKeyDir:   "/etc/zenex/backup",
	}
}

// Result is returned to the caller for operations that produce a value.
type Result struct {
	UID    string `json:"uid,omitempty"`
	Output string `json:"output,omitempty"`
}

// commandRunner is the subset of the executor the helper needs.
type commandRunner interface {
	Run(ctx context.Context, bin string, args []string, timeout time.Duration) (executor.Result, error)
	RunInput(ctx context.Context, bin string, args []string, stdin string, timeout time.Duration) (executor.Result, error)
}

// Ops executes validated operations. All operations are serialized by one lock,
// so two sites cannot change web server or PHP-FPM configuration at the same time.
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
	case "services.status":
		return o.servicesStatus(ctx)
	case "files.list", "files.read", "files.write", "files.mkdir", "files.delete":
		return o.filesOp(ctx, op, args)
	case "php.versions":
		return o.phpVersionsOutput(), nil
	case "php.restart":
		return Result{}, o.phpRestart(ctx, args)
	case "php.switch":
		return Result{}, o.phpSwitch(ctx, args)
	case "vhost.disable":
		return Result{}, o.vhostDisable(ctx, args)
	case "vhost.enable":
		return Result{}, o.vhostEnable(ctx, args)
	case "site.purge":
		return Result{}, o.sitePurge(ctx, args)
	case "logs.tail":
		return o.logsTail(args)
	case "wp.harden":
		return Result{}, o.wpHarden(ctx, args)
	case "wp.update":
		return Result{}, o.wpUpdate(ctx, args)
	case "backup.create":
		return o.backupCreate(ctx, args)
	case "backup.keygen":
		return o.backupKeygen(ctx, args)
	case "backup.test":
		return Result{}, o.backupTest(ctx, args)
	case "backup.upload":
		return Result{}, o.backupUpload(ctx, args)
	case "backup.delete":
		return Result{}, o.backupDelete(ctx, args)
	case "panel.version":
		return o.panelVersion(ctx)
	case "panel.latest":
		return o.panelLatest(ctx)
	case "panel.update-start":
		return o.updateStart(ctx)
	case "panel.update-status":
		return o.updateStatusOutput(ctx)
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
// document root; the web server group can read it; no other account can enter it.
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
		return fmt.Errorf("web group www-data missing: %w", err)
	}
	uid, _ := strconv.Atoi(siteUser.Uid)
	gid, _ := strconv.Atoi(web.Gid)

	if err := os.MkdirAll(o.docRoot(name), 0o750); err != nil {
		return fmt.Errorf("create document root: %w", err)
	}
	// The home directory only has to be traversable: the site account needs to
	// pass through it to reach htdocs, and the web server needs the same. It holds no
	// files itself, and htdocs is readable only by the site account and the web server,
	// so other sites cannot read it.
	if err := os.Chown(o.homeDir(name), 0, 0); err != nil {
		return err
	}
	if err := os.Chmod(o.homeDir(name), 0o751); err != nil {
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
	maintenance, err := maintenanceFlag(args)
	if err != nil {
		return err
	}
	avail := filepath.Join(o.Paths.CaddyAvailable, "zx-"+name+".caddy")
	enabled := filepath.Join(o.Paths.CaddyEnabled, "zx-"+name+".caddy")

	if err := writeFileAtomic(avail, []byte(vhostConfig(name, domain, o.docRoot(name), o.Paths.LogDir, maintenance)), 0o644); err != nil {
		return fmt.Errorf("write site config: %w", err)
	}
	_ = os.Remove(enabled)
	if err := os.Symlink(avail, enabled); err != nil {
		_ = os.Remove(avail)
		return fmt.Errorf("enable site config: %w", err)
	}
	// caddy.Reload validates the configuration first and never reloads a bad one.
	if err := caddy.Reload(ctx, o.Exec); err != nil {
		_ = os.Remove(enabled)
		_ = os.Remove(avail)
		return fmt.Errorf("web server rejected the site configuration; site not enabled: %w", err)
	}
	return nil
}

// maintenanceFlag reads the optional "maintenance" argument. Missing means off.
func maintenanceFlag(args map[string]string) (bool, error) {
	switch args["maintenance"] {
	case "", "0":
		return false, nil
	case "1":
		return true, nil
	}
	return false, errors.New("invalid maintenance flag")
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
// configuration file readable only by the site account and the web server.
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

// wpUpdate runs the WordPress core, plugin and theme updates as the site account.
// Each step may exit non-zero when there is nothing to update, so only a WP-CLI
// that cannot start is reported as a failure.
func (o *Ops) wpUpdate(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	if args["path"] != o.docRoot(name) {
		return errors.New("invalid website folder")
	}
	steps := [][]string{
		{"core", "update", "--quiet"},
		{"plugin", "update", "--all", "--quiet"},
		{"theme", "update", "--all", "--quiet"},
	}
	for _, step := range steps {
		if err := o.wpRunUpdate(ctx, name, step...); err != nil {
			return err
		}
	}
	return nil
}

// wpRunUpdate is wpRun with the update timeout, which ignores the WP-CLI exit code.
func (o *Ops) wpRunUpdate(ctx context.Context, linuxUser string, wpArgs ...string) error {
	home := o.homeDir(linuxUser)
	argv := []string{"-u", linuxUser, "--", binEnv, "HOME=" + home, binWP, "--path=" + o.docRoot(linuxUser)}
	argv = append(argv, wpArgs...)
	res, err := o.Exec.Run(ctx, binRunuser, argv, wpUpdateTime)
	if err != nil {
		return fmt.Errorf("wp-cli: %w", err)
	}
	if res.ExitCode == exitNotFound || res.ExitCode == exitNotExec {
		return fmt.Errorf("wp-cli cannot run: %s", trim(stderrOr(res, nil)))
	}
	return nil
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

// vhostConfig builds the site file. In maintenance mode the final handler answers
// every request with 503 instead of running PHP; the blocks above it still apply.
func vhostConfig(name, domain, docRoot, logDir string, maintenance bool) string {
	handler := fmt.Sprintf(`handle {
        php_fastcgi unix//run/php/zx-%s.sock
        file_server
    }`, name)
	if maintenance {
		handler = `handle {
        header Retry-After 3600
        header Content-Type "text/html; charset=utf-8"
        respond ` + "`" + `<!doctype html><html><head><meta charset=utf-8><title>Maintenance</title></head><body><h1>Under maintenance - back soon</h1></body></html>` + "`" + ` 503
    }`
	}
	return fmt.Sprintf(`# Managed by Zenex. Do not edit by hand.
%[2]s {
    root * %[3]s
    encode zstd gzip

    log {
        output file %[4]s/zx-%[1]s.log {
            roll_size 10mb
            roll_keep 3
        }
        format console
    }

    # PHP must never run from uploads, even if a file is planted there.
    @uploads_php path_regexp uploads (?i)/wp-content/uploads/.*\.php$
    handle @uploads_php {
        respond 403
    }

    # Hidden files such as .htaccess or .env are never served.
    @dotfiles path_regexp dotfiles (^|/)\.
    handle @dotfiles {
        respond 403
    }

    %[5]s
}
`, name, domain, docRoot, logDir, handler)
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
	base := []string{binUseradd, binUserdel, binRunuser, binEnv, binMariadb, binMariadbDump, binTar, binSystemctl, binWP, binGit, binSystemdRun, binBash, binSSHKeygen, binSFTP}
	base = append(base, caddy.AllowedBinaries...)
	return append(base, phpFPM...)
}
