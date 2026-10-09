package helper

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

const (
	cpanelPassword = "s3cret'pass\"word"
	sampleConfig   = `<?php
// define('DB_NAME', 'old_name');
define( 'DB_NAME', 'acct_wp1' );
define( 'DB_USER', 'acct_user' );
define( 'DB_PASSWORD', 'p\'ss\\word' );
define( 'DB_HOST', 'localhost' );
$table_prefix = 'xy_';
require_once ABSPATH . 'wp-settings.php';
`
)

func TestParseWPConfig(t *testing.T) {
	cfg, err := parseWPConfig(sampleConfig)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.name != "acct_wp1" || cfg.user != "acct_user" || cfg.host != "localhost" || cfg.prefix != "xy_" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.password != `p'ss\word` {
		t.Fatalf("password = %q", cfg.password)
	}
}

func TestParseWPConfigDoubleQuotesAndDefaults(t *testing.T) {
	cfg, err := parseWPConfig(`<?php
/* define('DB_NAME', 'commented'); */
define("DB_NAME", "db1");
define("DB_USER", "u1");
define("DB_PASSWORD", "");
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.name != "db1" || cfg.password != "" || cfg.host != "localhost" || cfg.prefix != "wp_" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestParseWPConfigRefusesWhatItCannotRead(t *testing.T) {
	cases := map[string]string{
		"no settings":   `<?php echo 1;`,
		"from env":      `<?php define('DB_NAME', getenv('DB')); define('DB_USER','u'); define('DB_PASSWORD','p');`,
		"only comments": "<?php\n// define('DB_NAME','a'); define('DB_USER','b'); define('DB_PASSWORD','c');\n",
		"odd prefix":    `<?php define('DB_NAME','a'); define('DB_USER','b'); define('DB_PASSWORD','c'); $table_prefix = 'a-b';`,
	}
	for name, text := range cases {
		// "odd prefix" does not match the prefix pattern and so falls back to wp_.
		_, err := parseWPConfig(text)
		if name == "odd prefix" {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"plain":    `'plain'`,
		"a'b":      `'a'\''b'`,
		"$(x) `y`": "'$(x) `y`'",
		"":         "''",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestSplitDBHost(t *testing.T) {
	host, port, err := splitDBHost("db.example.com:3307")
	if err != nil || host != "db.example.com" || port != "3307" {
		t.Fatalf("got %q %q %v", host, port, err)
	}
	if host, port, _ := splitDBHost("localhost:/var/run/mysqld/mysqld.sock"); host != "localhost" || port != "" {
		t.Fatalf("socket form: %q %q", host, port)
	}
	if host, _, _ := splitDBHost(""); host != "localhost" {
		t.Fatalf("empty host = %q", host)
	}
	if _, _, err := splitDBHost("bad host;rm"); err == nil {
		t.Fatal("unsafe host accepted")
	}
}

func TestMysqlCommandKeepsPasswordOutOfArguments(t *testing.T) {
	cfg := wpConfig{name: "db", user: "u", password: `p'w"d`, host: "localhost:3306", prefix: "wp_"}
	cmd, err := mysqlCommand(cfg, "mysqldump", "--quick")
	if err != nil {
		t.Fatal(err)
	}
	want := `MYSQL_PWD='p'\''w"d' mysqldump --quick -h 'localhost' -P 3306 -u 'u' 'db'`
	if cmd != want {
		t.Fatalf("cmd = %s\nwant  %s", cmd, want)
	}
	if strings.Count(cmd, "p'") > 1 && strings.Contains(strings.TrimPrefix(cmd, "MYSQL_PWD="), `w"d`) {
		t.Fatal("password appears outside MYSQL_PWD")
	}
}

func TestParseFindOutput(t *testing.T) {
	out := strings.Join([]string{
		"/home/acct/public_html/wp-config.php",
		"/home/acct/public_html/wp-config.php",   // duplicate
		"/home/acct/sites/my blog/wp-config.php", // a space: not supported by the transfer rules
		"/home/acct/addon/wp-config.php",
		"relative/wp-config.php",
		"/wp-config.php",
		"/home/acct/public_html/other.php",
		"",
	}, "\n")
	got := parseFindOutput(out)
	if len(got) != 2 || got[0] != "/home/acct/public_html/wp-config.php" || got[1] != "/home/acct/addon/wp-config.php" {
		t.Fatalf("paths = %v", got)
	}
}

// cpanelExec answers the ssh commands of a fake cPanel account and writes the streamed files.
type cpanelExec struct {
	t         *testing.T
	sshCmds   []string
	envs      [][]string
	argvs     [][]string
	dumpTail  string // what the fake mysqldump ends with
	dumpFails bool
}

func (c *cpanelExec) Run(_ context.Context, bin string, args []string, _ time.Duration) (executor.Result, error) {
	if bin == binTar && len(args) > 0 {
		switch args[0] {
		case "-xzf": // unpack the streamed archive into htdocs
			dir := args[indexOfArg(args, "-C")+1]
			_ = os.WriteFile(filepath.Join(dir, "index.php"), []byte("<?php"), 0o600)
		case "-czf": // pack the final archive
			_ = os.WriteFile(args[1], []byte("archive-bytes"), 0o600)
		}
	}
	return executor.Result{}, nil
}

func (c *cpanelExec) RunInput(ctx context.Context, bin string, args []string, _ string, timeout time.Duration) (executor.Result, error) {
	return c.Run(ctx, bin, args, timeout)
}

func (c *cpanelExec) RunEnv(_ context.Context, bin string, args []string, env []string, _ time.Duration) (executor.Result, error) {
	cmd := args[len(args)-1]
	c.sshCmds = append(c.sshCmds, cmd)
	c.envs = append(c.envs, env)
	c.argvs = append(c.argvs, append([]string{bin}, args...))
	switch {
	case strings.HasPrefix(cmd, "find "):
		return executor.Result{Stdout: "/home/acct/public_html/wp-config.php\n"}, nil
	case strings.HasPrefix(cmd, "head -c"):
		return executor.Result{Stdout: sampleConfig}, nil
	case strings.Contains(cmd, "mysql -N -B"):
		return executor.Result{Stdout: "https://www.example.com\n"}, nil
	case strings.HasPrefix(cmd, "du -sk"):
		return executor.Result{Stdout: "2048\t/home/acct/public_html\n"}, nil
	}
	return executor.Result{}, nil
}

func (c *cpanelExec) RunToFile(_ context.Context, bin string, args []string, env []string, outPath string, _ time.Duration) (executor.Result, error) {
	cmd := args[len(args)-1]
	c.sshCmds = append(c.sshCmds, cmd)
	c.envs = append(c.envs, env)
	c.argvs = append(c.argvs, append([]string{bin}, args...))
	switch {
	case strings.HasPrefix(cmd, "tar -czf -"):
		return executor.Result{}, os.WriteFile(outPath, []byte("tar-stream"), 0o600)
	case strings.Contains(cmd, "mysqldump"):
		if c.dumpFails {
			return executor.Result{ExitCode: 2, Stderr: "mysqldump: Got error: Access denied"}, nil
		}
		return executor.Result{}, os.WriteFile(outPath, []byte("CREATE TABLE a (b int);\n"+c.dumpTail), 0o600)
	}
	return executor.Result{ExitCode: 1, Stderr: "unexpected command"}, nil
}

func cpanelOps(t *testing.T, ex commandRunner) (*Ops, string) {
	t.Helper()
	root := t.TempDir()
	return &Ops{Exec: ex, Paths: Paths{
		WebRoot:      filepath.Join(root, "www"),
		BackupDir:    filepath.Join(root, "backups"),
		BackupKeyDir: filepath.Join(root, "key"),
	}}, root
}

func cpanelArgs(extra map[string]string) map[string]string {
	args := map[string]string{
		"host": "cpanel.example.net", "port": "22", "username": "acct", "password": cpanelPassword,
	}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

func TestCpanelScanReportsInstalls(t *testing.T) {
	ex := &cpanelExec{t: t}
	o, _ := cpanelOps(t, ex)

	res, err := o.Do(context.Background(), "cpanel.scan", cpanelArgs(nil))
	if err != nil {
		t.Fatal(err)
	}
	var found []CpanelInstall
	if err := json.Unmarshal([]byte(res.Output), &found); err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("found = %+v", found)
	}
	got := found[0]
	if got.Path != "/home/acct/public_html" || got.Domain != "www.example.com" || got.DBName != "acct_wp1" || got.Prefix != "xy_" || got.SizeKB != 2048 {
		t.Fatalf("install = %+v", got)
	}
	if strings.Contains(res.Output, "acct_user") || strings.Contains(res.Output, "ss") && strings.Contains(res.Output, "word") {
		t.Fatalf("scan output carries database credentials: %s", res.Output)
	}
}

func TestCpanelPasswordGoesInEnvironmentOnly(t *testing.T) {
	ex := &cpanelExec{t: t}
	o, _ := cpanelOps(t, ex)
	if _, err := o.Do(context.Background(), "cpanel.scan", cpanelArgs(nil)); err != nil {
		t.Fatal(err)
	}
	if len(ex.argvs) == 0 {
		t.Fatal("no ssh command ran")
	}
	for i, argv := range ex.argvs {
		if strings.Contains(strings.Join(argv, " "), cpanelPassword) {
			t.Fatalf("password in argv: %v", argv)
		}
		if len(ex.envs[i]) != 1 || ex.envs[i][0] != "SSHPASS="+cpanelPassword {
			t.Fatalf("env = %v", ex.envs[i])
		}
		if argv[0] != binSSHPass || argv[1] != "-e" || argv[2] != binSSH {
			t.Fatalf("argv = %v", argv)
		}
	}
}

func pullArgs(root string, extra map[string]string) map[string]string {
	args := cpanelArgs(map[string]string{
		"path":   "/home/acct/public_html",
		"user":   "zx_shop",
		"output": filepath.Join(root, "backups", "zx_shop", "migrate-1.tar.gz"),
	})
	for k, v := range extra {
		args[k] = v
	}
	return args
}

func TestCpanelPullBuildsArchiveAndLeavesNoWorkFiles(t *testing.T) {
	ex := &cpanelExec{t: t, dumpTail: "-- Dump completed on 2026-10-09\n"}
	o, root := cpanelOps(t, ex)
	args := pullArgs(root, nil)

	res, err := o.Do(context.Background(), "cpanel.pull", args)
	if err != nil {
		t.Fatal(err)
	}
	if want := args["output"] + " " + strconv.Itoa(len("archive-bytes")); res.Output != want {
		t.Fatalf("output = %q, want %q", res.Output, want)
	}
	entries, _ := os.ReadDir(filepath.Dir(args["output"]))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "migrate-") && e.IsDir() {
			t.Fatalf("work folder left behind: %s", e.Name())
		}
	}

	var tarCmd, dumpCmd string
	for _, c := range ex.sshCmds {
		if strings.HasPrefix(c, "tar -czf -") {
			tarCmd = c
		}
		if strings.Contains(c, "mysqldump") {
			dumpCmd = c
		}
	}
	if !strings.Contains(tarCmd, "--exclude='./wp-content/cache'") || !strings.HasSuffix(tarCmd, "-C '/home/acct/public_html' .") {
		t.Fatalf("tar command = %s", tarCmd)
	}
	if !strings.HasPrefix(dumpCmd, `MYSQL_PWD='p'\''ss\word' mysqldump `) || !strings.HasSuffix(dumpCmd, "-u 'acct_user' 'acct_wp1'") {
		t.Fatalf("dump command = %s", dumpCmd)
	}
}

func TestCpanelPullRefusesATruncatedDump(t *testing.T) {
	ex := &cpanelExec{t: t, dumpTail: "INSERT INTO a VALUES (1);\n"}
	o, root := cpanelOps(t, ex)
	args := pullArgs(root, nil)

	if _, err := o.Do(context.Background(), "cpanel.pull", args); err == nil {
		t.Fatal("a dump without its closing line was accepted")
	}
	if _, err := os.Stat(args["output"]); err == nil {
		t.Fatal("an archive was left after a failed pull")
	}
}

func TestCpanelPullReportsDumpFailureWithoutThePassword(t *testing.T) {
	ex := &cpanelExec{t: t, dumpFails: true}
	o, root := cpanelOps(t, ex)
	_, err := o.Do(context.Background(), "cpanel.pull", pullArgs(root, nil))
	if err == nil {
		t.Fatal("a failed dump was accepted")
	}
	if strings.Contains(err.Error(), cpanelPassword) || strings.Contains(err.Error(), "ss\\word") {
		t.Fatalf("error carries a password: %v", err)
	}
}

func TestCpanelPullRefusesBadInputBeforeConnecting(t *testing.T) {
	cases := map[string]map[string]string{
		"root folder":     {"path": "/"},
		"path traversal":  {"path": "/home/acct/../etc"},
		"system account":  {"user": "root"},
		"other site dir":  {"output": "OTHER"},
		"empty password":  {"password": ""},
		"bad host":        {"host": "bad host"},
		"output suffix":   {"output": "SUFFIX"},
		"newline in path": {"path": "/home/acct/x\ny"},
	}
	for name, extra := range cases {
		ex := &cpanelExec{t: t}
		o, root := cpanelOps(t, ex)
		switch extra["output"] {
		case "OTHER":
			extra["output"] = filepath.Join(root, "backups", "zx_other", "migrate-1.tar.gz")
		case "SUFFIX":
			extra["output"] = filepath.Join(root, "backups", "zx_shop", "migrate-1.zip")
		}
		if _, err := o.Do(context.Background(), "cpanel.pull", pullArgs(root, extra)); err == nil {
			t.Errorf("%s accepted", name)
		}
		if len(ex.sshCmds) != 0 {
			t.Errorf("%s: ssh ran before the input was checked: %v", name, ex.sshCmds)
		}
	}
}

func TestRemoveSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symbolic links unavailable here: %v", err)
	}
	if err := removeSymlinks(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "link")); err == nil {
		t.Fatal("link kept")
	}
	if _, err := os.Stat(filepath.Join(dir, "real.txt")); err != nil {
		t.Fatal("regular file removed")
	}
}
