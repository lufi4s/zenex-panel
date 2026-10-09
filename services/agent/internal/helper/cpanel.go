package helper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

// Migration from a cPanel account. Everything is read-only on the cPanel side: the helper signs
// in over SSH with the account's password, lists WordPress installs, and streams a folder
// archive and a database dump to this server. Nothing is written, changed or left behind there.

const (
	binSSH = "/usr/bin/ssh"

	cpanelCmdTime  = 2 * time.Minute
	cpanelFindTime = 3 * time.Minute
	cpanelPullTime = 3 * time.Hour

	// maxInstalls caps how many WordPress installs one scan reports.
	maxInstalls = 20
	// maxWPConfigBytes is read from a remote wp-config.php; real files are a few KB.
	maxWPConfigBytes = 60 << 10

	msgSSHFailed = "could not run a command on the cPanel server"
)

var (
	dbHostRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,252}$`)
	prefixRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,32}$`)

	blockComments = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineComments  = regexp.MustCompile(`(?m)^[ \t]*(?://|#).*$`)
	prefixDef     = regexp.MustCompile(`\$table_prefix\s*=\s*(?:'([A-Za-z0-9_]*)'|"([A-Za-z0-9_]*)")`)
)

// wpConfig holds the database settings read from a wp-config.php.
type wpConfig struct {
	name, user, password, host, prefix string
}

// constDef returns a pattern that matches define('NAME', 'value') with either quote style.
func constDef(name string) *regexp.Regexp {
	return regexp.MustCompile(`define\s*\(\s*['"]` + name + `['"]\s*,\s*(?:'((?:\\.|[^'\\])*)'|"((?:\\.|[^"\\])*)")\s*\)`)
}

var (
	defName = constDef("DB_NAME")
	defUser = constDef("DB_USER")
	defPass = constDef("DB_PASSWORD")
	defHost = constDef("DB_HOST")
)

// parseWPConfig reads the database settings from the text of a wp-config.php. Comments are
// ignored first, so an old commented-out block never wins. A value that is not a plain string
// (for example read from the environment) is an error, because it cannot be known here.
func parseWPConfig(text string) (wpConfig, error) {
	text = blockComments.ReplaceAllString(text, "")
	text = lineComments.ReplaceAllString(text, "")

	// The first group is a single-quoted value, the second a double-quoted one. Which group
	// matched decides the escape rules, so an empty value is still told apart from a missing one.
	get := func(re *regexp.Regexp) (string, bool) {
		loc := re.FindStringSubmatchIndex(text)
		if loc == nil {
			return "", false
		}
		if loc[2] >= 0 {
			return unescapeSingle(text[loc[2]:loc[3]]), true
		}
		return unescapeDouble(text[loc[4]:loc[5]]), true
	}
	var cfg wpConfig
	var ok bool
	if cfg.name, ok = get(defName); !ok || cfg.name == "" {
		return wpConfig{}, errors.New("DB_NAME was not found in wp-config.php")
	}
	if cfg.user, ok = get(defUser); !ok || cfg.user == "" {
		return wpConfig{}, errors.New("DB_USER was not found in wp-config.php")
	}
	if cfg.password, ok = get(defPass); !ok {
		return wpConfig{}, errors.New("DB_PASSWORD was not found in wp-config.php")
	}
	cfg.host, _ = get(defHost)
	if cfg.host == "" {
		cfg.host = "localhost"
	}
	cfg.prefix = "wp_"
	if m := prefixDef.FindStringSubmatch(text); m != nil {
		if p := m[1] + m[2]; p != "" {
			cfg.prefix = p
		}
	}
	for _, v := range []string{cfg.name, cfg.user, cfg.password, cfg.host} {
		if strings.ContainsFunc(v, isControl) {
			return wpConfig{}, errors.New("wp-config.php contains characters that cannot be used")
		}
	}
	if !prefixRe.MatchString(cfg.prefix) {
		return wpConfig{}, errors.New("the table prefix in wp-config.php is not supported")
	}
	return cfg, nil
}

// unescapeSingle undoes PHP's single-quote escapes: only \\ and \' are special.
func unescapeSingle(s string) string {
	s = strings.ReplaceAll(s, `\\`, "\x00")
	s = strings.ReplaceAll(s, `\'`, `'`)
	return strings.ReplaceAll(s, "\x00", `\`)
}

// unescapeDouble undoes the PHP double-quote escapes that matter in a password.
func unescapeDouble(s string) string {
	s = strings.ReplaceAll(s, `\\`, "\x00")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\$`, `$`)
	return strings.ReplaceAll(s, "\x00", `\`)
}

// shellQuote wraps s in single quotes for a POSIX shell. Every value that comes from the
// remote side goes through here before it becomes part of a command line.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// splitDBHost reads DB_HOST. "localhost:3306" gives a host and a port. A socket path after the
// colon is ignored, so the default socket is used.
func splitDBHost(raw string) (host, port string, err error) {
	host, port, _ = strings.Cut(strings.TrimSpace(raw), ":")
	if host == "" {
		host = "localhost"
	}
	if !dbHostRe.MatchString(host) {
		return "", "", errors.New("the database host in wp-config.php is not supported")
	}
	if port != "" {
		if n, convErr := strconv.Atoi(port); convErr != nil || n < 1 || n > 65535 {
			port = ""
		}
	}
	return host, port, nil
}

// mysqlCommand builds "MYSQL_PWD=... <tool> <flags> -h host [-P port] -u user db". The password
// is passed in the environment of that one remote command, never as an argument.
func mysqlCommand(cfg wpConfig, tool string, flags ...string) (string, error) {
	host, port, err := splitDBHost(cfg.host)
	if err != nil {
		return "", err
	}
	parts := []string{"MYSQL_PWD=" + shellQuote(cfg.password), tool}
	parts = append(parts, flags...)
	parts = append(parts, "-h", shellQuote(host))
	if port != "" {
		parts = append(parts, "-P", port)
	}
	parts = append(parts, "-u", shellQuote(cfg.user), shellQuote(cfg.name))
	return strings.Join(parts, " "), nil
}

// ---------------------------------------------------------------------------
// SSH
// ---------------------------------------------------------------------------

// cpanelTarget reads and checks the connection arguments shared by scan and pull.
func cpanelTarget(args map[string]string) (remoteTarget, string, error) {
	t, err := parseRemoteTarget(args)
	if err != nil {
		return remoteTarget{}, "", err
	}
	pw := args["password"]
	if err := validatePassword(pw); err != nil {
		return remoteTarget{}, "", err
	}
	return t, pw, nil
}

// sshArgv is the argument list for "sshpass -e ssh ... user@host command".
func (o *Ops) sshArgv(t remoteTarget, command string) ([]string, error) {
	keyPath, err := o.backupKeyPath()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, errors.New("prepare connection failed")
	}
	return []string{
		"-e", binSSH,
		"-p", strconv.Itoa(t.port),
		"-o", "PubkeyAuthentication=no",
		"-o", "PreferredAuthentications=password,keyboard-interactive",
		"-o", "NumberOfPasswordPrompts=1",
		"-o", "BatchMode=no",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + filepath.Join(dir, knownHostsName),
		"-o", "ConnectTimeout=15",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=6",
		"-T",
		t.username + "@" + t.host,
		command,
	}, nil
}

// sshFailure turns a finished ssh run into a short message. The password is removed from
// anything that is quoted back.
func sshFailure(res executor.Result, password, def string) error {
	if res.ExitCode == 255 || res.ExitCode == sshpassWrongPassword || res.ExitCode == sshpassHostKeyUnknown || res.ExitCode == sshpassHostKeyChanged {
		if r := sftpReasonFor(res, true); r != "" {
			return errors.New(r)
		}
		return errors.New(msgConnect)
	}
	text := strings.TrimSpace(res.Stderr)
	if password != "" {
		text = strings.ReplaceAll(text, password, "[redacted]")
	}
	if text == "" {
		return errors.New(def)
	}
	if len(text) > 240 {
		text = text[:240] + "..."
	}
	return fmt.Errorf("%s: %s", def, text)
}

// runSSH runs one remote command and returns its output. A non-zero exit is an error.
func (o *Ops) runSSH(ctx context.Context, t remoteTarget, password, command string, timeout time.Duration) (string, error) {
	argv, err := o.sshArgv(t, command)
	if err != nil {
		return "", err
	}
	res, err := o.Exec.RunEnv(ctx, binSSHPass, argv, []string{"SSHPASS=" + password}, timeout)
	if err != nil {
		if isTimeout(err) {
			return "", errors.New(msgTimedOut)
		}
		return "", errors.New(msgSSHFailed)
	}
	if res.ExitCode != 0 {
		return "", sshFailure(res, password, msgSSHFailed)
	}
	return res.Stdout, nil
}

// runSSHToFile is runSSH with the output streamed to a new local file.
func (o *Ops) runSSHToFile(ctx context.Context, t remoteTarget, password, command, outPath string, timeout time.Duration) (executor.Result, error) {
	argv, err := o.sshArgv(t, command)
	if err != nil {
		return executor.Result{}, err
	}
	res, err := o.Exec.RunToFile(ctx, binSSHPass, argv, []string{"SSHPASS=" + password}, outPath, timeout)
	if err != nil {
		if isTimeout(err) {
			return res, errors.New(msgTimedOut)
		}
		return res, errors.New(msgSSHFailed)
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// cpanel.scan
// ---------------------------------------------------------------------------

// CpanelInstall is one WordPress install found on the cPanel account.
type CpanelInstall struct {
	Path    string `json:"path"`     // the website folder, for example /home/user/public_html
	SiteURL string `json:"site_url"` // the address stored in the database, when it could be read
	Domain  string `json:"domain"`   // the host of SiteURL
	DBName  string `json:"db_name"`
	Prefix  string `json:"table_prefix"`
	SizeKB  int64  `json:"size_kb"`
}

// findCommand lists wp-config.php files in the account's home folder, without hidden folders
// and without plugin or theme test folders. It is a fixed string: nothing from the caller is in it.
const findCommand = `find "$HOME" -maxdepth 6 -type f -name wp-config.php -not -path '*/.*/*' -not -path '*/wp-content/*' -not -path '*/node_modules/*' 2>/dev/null | head -100`

// parseFindOutput keeps the safe, absolute wp-config.php paths from the find output.
func parseFindOutput(out string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] || path.Base(line) != "wp-config.php" {
			continue
		}
		if validateRemotePath(line) != nil || path.Dir(line) == "/" {
			continue
		}
		seen[line] = true
		paths = append(paths, line)
		if len(paths) >= maxInstalls {
			break
		}
	}
	return paths
}

// cpanelScan finds the WordPress installs of an account. For each one it reads the database
// settings from wp-config.php and asks the database for the website address. Failures of those
// two lookups leave the fields empty; they do not hide the install.
func (o *Ops) cpanelScan(ctx context.Context, args map[string]string) (Result, error) {
	t, pw, err := cpanelTarget(args)
	if err != nil {
		return Result{}, err
	}
	out, err := o.runSSH(ctx, t, pw, findCommand, cpanelFindTime)
	if err != nil {
		return Result{}, err
	}

	found := []CpanelInstall{}
	for _, cfgPath := range parseFindOutput(out) {
		dir := path.Dir(cfgPath)
		text, err := o.runSSH(ctx, t, pw, "head -c "+strconv.Itoa(maxWPConfigBytes)+" "+shellQuote(cfgPath), cpanelCmdTime)
		if err != nil {
			continue
		}
		cfg, err := parseWPConfig(text)
		if err != nil {
			continue
		}
		install := CpanelInstall{Path: dir, DBName: cfg.name, Prefix: cfg.prefix}
		if siteURL := o.lookupSiteURL(ctx, t, pw, cfg); siteURL != "" {
			install.SiteURL = siteURL
			install.Domain = hostOfURL(siteURL)
		}
		if du, err := o.runSSH(ctx, t, pw, "du -sk -- "+shellQuote(dir)+" 2>/dev/null | head -1", cpanelCmdTime); err == nil {
			if f := strings.Fields(du); len(f) > 0 {
				install.SizeKB, _ = strconv.ParseInt(f[0], 10, 64)
			}
		}
		found = append(found, install)
	}
	data, err := json.Marshal(found)
	if err != nil {
		return Result{}, errors.New("encode scan failed")
	}
	return Result{Output: string(data)}, nil
}

// lookupSiteURL reads the "home" address from the website's own database. It returns "" when
// the database cannot be reached from the cPanel server's command line.
func (o *Ops) lookupSiteURL(ctx context.Context, t remoteTarget, pw string, cfg wpConfig) string {
	query := "SELECT option_value FROM " + cfg.prefix + "options WHERE option_name IN ('home','siteurl') ORDER BY option_name='home' DESC LIMIT 1"
	cmd, err := mysqlCommand(cfg, "mysql", "-N", "-B")
	if err != nil {
		return ""
	}
	out, err := o.runSSH(ctx, t, pw, cmd+" -e "+shellQuote(query), cpanelCmdTime)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
}

// hostOfURL returns the lower-case host of an address, or "".
func hostOfURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// ---------------------------------------------------------------------------
// cpanel.pull
// ---------------------------------------------------------------------------

// pullExcludes are folders that hold caches and other backups, not the website itself.
var pullExcludes = []string{
	"./error_log",
	"./wp-content/cache",
	"./wp-content/upgrade",
	"./wp-content/updraft",
	"./wp-content/ai1wm-backups",
}

// cpanelPull copies a WordPress website from the cPanel account into a standard archive
// (htdocs/ and database.sql) in the site's backup folder, ready for backup.restore.
// Reply: "<archive path> <size in bytes>".
func (o *Ops) cpanelPull(ctx context.Context, args map[string]string) (Result, error) {
	t, pw, err := cpanelTarget(args)
	if err != nil {
		return Result{}, err
	}
	remoteDir := args["path"]
	if err := validateRemotePath(remoteDir); err != nil || remoteDir == "/" {
		return Result{}, errors.New("invalid website folder")
	}
	name, err := requireLinuxUser(args)
	if err != nil {
		return Result{}, err
	}
	output, err := o.backupPath(args["output"])
	if err != nil {
		return Result{}, err
	}
	siteDir := filepath.Join(filepath.Clean(o.Paths.BackupDir), name)
	if !strings.HasPrefix(output, siteDir+string(filepath.Separator)) {
		return Result{}, errors.New("the archive must be stored in the site's backup folder")
	}
	if exists, err := pathExists(output); err != nil || exists {
		return Result{}, errors.New("an archive with this name already exists")
	}
	if err := os.MkdirAll(siteDir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create backup folder: %w", err)
	}
	work, err := os.MkdirTemp(siteDir, "migrate-")
	if err != nil {
		return Result{}, fmt.Errorf("create work folder: %w", err)
	}
	done := false
	defer func() {
		_ = os.RemoveAll(work)
		if !done {
			_ = os.Remove(output)
		}
	}()

	// 1. The database settings.
	text, err := o.runSSH(ctx, t, pw, "head -c "+strconv.Itoa(maxWPConfigBytes)+" "+shellQuote(path.Join(remoteDir, "wp-config.php")), cpanelCmdTime)
	if err != nil {
		return Result{}, fmt.Errorf("read wp-config.php: %w", err)
	}
	cfg, err := parseWPConfig(text)
	if err != nil {
		return Result{}, err
	}

	// 2. The files, streamed as one archive.
	htdocs := filepath.Join(work, "htdocs")
	if err := os.Mkdir(htdocs, 0o700); err != nil {
		return Result{}, errors.New("create work folder failed")
	}
	if err := o.pullFiles(ctx, t, pw, remoteDir, work, htdocs); err != nil {
		return Result{}, err
	}

	// 3. The database, streamed as SQL. A dump without its closing line is cut short.
	sqlPath := filepath.Join(work, "database.sql")
	if err := o.pullDatabase(ctx, t, pw, cfg, sqlPath); err != nil {
		return Result{}, err
	}

	// 4. Both, in the layout backup.create uses.
	res, err := o.Exec.Run(ctx, binTar, []string{"-czf", output, "-C", work, "htdocs", "database.sql"}, backupTarTime)
	if err := execErr(res, err); err != nil {
		return Result{}, fmt.Errorf("archive failed: %s", trim(err.Error()))
	}
	if err := os.Chmod(output, 0o600); err != nil {
		return Result{}, err
	}
	info, err := os.Stat(output)
	if err != nil {
		return Result{}, fmt.Errorf("archive missing after packing: %w", err)
	}
	done = true
	return Result{Output: output + " " + strconv.FormatInt(info.Size(), 10)}, nil
}

// pullFiles streams "tar -czf -" from the remote folder, unpacks it into htdocs and removes
// symbolic links, which the restore refuses. Exit code 1 from tar means a file changed while
// it was read, which is normal on a live website.
func (o *Ops) pullFiles(ctx context.Context, t remoteTarget, pw, remoteDir, work, htdocs string) error {
	var cmd strings.Builder
	cmd.WriteString("tar -czf -")
	for _, ex := range pullExcludes {
		cmd.WriteString(" --exclude=" + shellQuote(ex))
	}
	cmd.WriteString(" -C " + shellQuote(remoteDir) + " .")

	archive := filepath.Join(work, "files.tar.gz")
	res, err := o.runSSHToFile(ctx, t, pw, cmd.String(), archive, cpanelPullTime)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 && res.ExitCode != 1 {
		return sshFailure(res, pw, "the website files could not be copied")
	}
	if info, err := os.Stat(archive); err != nil || info.Size() == 0 {
		return errors.New("the website files could not be copied")
	}
	untar, err := o.Exec.Run(ctx, binTar, []string{"-xzf", archive, "-C", htdocs, "--no-same-owner"}, backupTarTime)
	if err := execErr(untar, err); err != nil {
		return fmt.Errorf("the copied files are incomplete: %s", trim(err.Error()))
	}
	_ = os.Remove(archive)
	return removeSymlinks(htdocs)
}

// removeSymlinks deletes every symbolic link below dir. A link could point outside the website.
func removeSymlinks(dir string) error {
	var links []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			links = append(links, p)
		}
		return nil
	})
	if err != nil {
		return errors.New("the copied files could not be checked")
	}
	for _, l := range links {
		if err := os.Remove(l); err != nil {
			return errors.New("a symbolic link could not be removed")
		}
	}
	return nil
}

// dumpFlagSets are tried in order: the second is for older servers that reject the first set.
var dumpFlagSets = [][]string{
	{"--single-transaction", "--quick", "--no-tablespaces", "--default-character-set=utf8mb4"},
	{"--single-transaction", "--quick"},
}

// pullDatabase streams mysqldump from the remote server into sqlPath.
func (o *Ops) pullDatabase(ctx context.Context, t remoteTarget, pw string, cfg wpConfig, sqlPath string) error {
	var lastErr error
	for _, flags := range dumpFlagSets {
		cmd, err := mysqlCommand(cfg, "mysqldump", flags...)
		if err != nil {
			return err
		}
		_ = os.Remove(sqlPath)
		res, err := o.runSSHToFile(ctx, t, pw, cmd, sqlPath, cpanelPullTime)
		if err != nil {
			return err
		}
		if res.ExitCode == 0 {
			if dumpComplete(sqlPath) {
				return nil
			}
			return errors.New("the database copy was cut short")
		}
		lastErr = sshFailure(redactSecret(res, cfg.password), pw, "the database could not be copied")
		lower := strings.ToLower(res.Stderr)
		if !strings.Contains(lower, "unknown option") && !strings.Contains(lower, "unrecognized option") && !strings.Contains(lower, "unknown variable") {
			break
		}
	}
	return lastErr
}

// redactSecret removes a secret from the stderr of a result.
func redactSecret(res executor.Result, secret string) executor.Result {
	if secret != "" {
		res.Stderr = strings.ReplaceAll(res.Stderr, secret, "[redacted]")
	}
	return res
}

// dumpComplete reports whether a mysqldump file ends with its closing comment.
func dumpComplete(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return false
	}
	const tail = 1024
	start := info.Size() - tail
	if start < 0 {
		start = 0
	}
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil {
		return false
	}
	return strings.Contains(string(buf), "-- Dump completed")
}
