package helper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
	"github.com/zenexcloud/zenex-panel/services/agent/internal/nginx"
)

const (
	binUserdel   = "/usr/sbin/userdel"
	logTailMax   = 64 << 10
	logTailLines = 200
)

// phpVersions returns the installed PHP-FPM versions, sorted, e.g. ["8.3"].
func (o *Ops) phpVersions() []string {
	matches, _ := filepath.Glob(o.Paths.PHPFPMGlob)
	var out []string
	for _, m := range matches {
		v := strings.TrimPrefix(filepath.Base(m), "php-fpm")
		if phpVerRe.MatchString(v) {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func (o *Ops) phpVersionsOutput() Result {
	return Result{Output: strings.Join(o.phpVersions(), ",")}
}

// phpRestart restarts the PHP-FPM service for one version. All sites on that
// version are affected for a moment, so the panel reports this to the customer.
func (o *Ops) phpRestart(ctx context.Context, args map[string]string) error {
	ver := args["php"]
	if err := requirePHPVersion(ver); err != nil {
		return err
	}
	res, err := o.Exec.Run(ctx, binSystemctl, []string{"restart", "php" + ver + "-fpm"}, 2*time.Minute)
	if err != nil || res.ExitCode != 0 {
		return fmt.Errorf("restart PHP %s failed: %s", ver, trim(stderrOr(res, err)))
	}
	return nil
}

// phpSwitch moves a site's PHP pool from one installed version to another.
func (o *Ops) phpSwitch(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	from, to := args["from"], args["to"]
	if err := requirePHPVersion(from); err != nil {
		return err
	}
	if err := requirePHPVersion(to); err != nil {
		return err
	}
	if from == to {
		return errors.New("the site already uses this PHP version")
	}
	if !contains(o.phpVersions(), to) {
		return fmt.Errorf("PHP %s is not installed on this server", to)
	}

	// Write the new pool first and prove it is valid before touching the old one.
	newDir := fmt.Sprintf(o.Paths.PHPPoolDir, to)
	newPath := filepath.Join(newDir, "zx-"+name+".conf")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(newPath, []byte(poolConfig(name, o.homeDir(name))), 0o644); err != nil {
		return fmt.Errorf("write pool: %w", err)
	}
	test, err := o.Exec.Run(ctx, "/usr/sbin/php-fpm"+to, []string{"-t"}, 30*time.Second)
	if err != nil || test.ExitCode != 0 {
		_ = os.Remove(newPath)
		return fmt.Errorf("PHP %s configuration test failed: %s", to, trim(stderrOr(test, err)))
	}
	if res, err := o.Exec.Run(ctx, binSystemctl, []string{"reload", "php" + to + "-fpm"}, time.Minute); err != nil || res.ExitCode != 0 {
		_ = os.Remove(newPath)
		return fmt.Errorf("reload PHP %s failed: %s", to, trim(stderrOr(res, err)))
	}

	oldPath := filepath.Join(fmt.Sprintf(o.Paths.PHPPoolDir, from), "zx-"+name+".conf")
	if err := os.Remove(oldPath); err == nil {
		_, _ = o.Exec.Run(ctx, binSystemctl, []string{"reload", "php" + from + "-fpm"}, time.Minute)
	}
	return nil
}

// vhostDisable takes a site offline by removing its enabled nginx config.
func (o *Ops) vhostDisable(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	enabled := filepath.Join(o.Paths.NginxEnabled, "zx-"+name+".conf")
	if err := os.Remove(enabled); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := nginx.Reload(ctx, o.Exec); err != nil {
		return fmt.Errorf("nginx rejected the configuration after disabling the site: %w", err)
	}
	return nil
}

// vhostEnable brings a suspended site back online.
func (o *Ops) vhostEnable(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	avail := filepath.Join(o.Paths.NginxAvailable, "zx-"+name+".conf")
	enabled := filepath.Join(o.Paths.NginxEnabled, "zx-"+name+".conf")
	if _, err := os.Stat(avail); err != nil {
		return errors.New("the site configuration is missing; rebuild the website")
	}
	_ = os.Remove(enabled)
	if err := os.Symlink(avail, enabled); err != nil {
		return err
	}
	if err := nginx.Reload(ctx, o.Exec); err != nil {
		_ = os.Remove(enabled)
		return fmt.Errorf("nginx rejected the configuration; site stays offline: %w", err)
	}
	return nil
}

// logsTail returns the end of a site's nginx error log.
func (o *Ops) logsTail(args map[string]string) (Result, error) {
	name, err := requireLinuxUser(args)
	if err != nil {
		return Result{}, err
	}
	path := filepath.Join(o.Paths.LogDir, "zx-"+name+".error.log")
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Result{Output: "No errors recorded yet."}, nil
	}
	if err != nil {
		return Result{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Result{}, err
	}
	start := int64(0)
	if info.Size() > logTailMax {
		start = info.Size() - logTailMax
	}
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return Result{}, err
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if len(lines) > logTailLines {
		lines = lines[len(lines)-logTailLines:]
	}
	return Result{Output: strings.Join(lines, "\n")}, nil
}

// sitePurge removes everything a site owns: nginx and PHP-FPM config, the
// database and its user, the files and the system account. Every step is safe to
// repeat, so a failed purge can be retried.
func (o *Ops) sitePurge(ctx context.Context, args map[string]string) error {
	name, err := requireLinuxUser(args)
	if err != nil {
		return err
	}
	db, dbUser := args["db"], args["dbuser"]
	if db != name || dbUser != name {
		return errors.New("database identifiers must match the site account")
	}

	var problems []string
	note := func(step string, err error) {
		if err != nil {
			problems = append(problems, step+": "+err.Error())
		}
	}

	// 1. Take the site offline.
	_ = os.Remove(filepath.Join(o.Paths.NginxEnabled, "zx-"+name+".conf"))
	_ = os.Remove(filepath.Join(o.Paths.NginxAvailable, "zx-"+name+".conf"))
	note("nginx", nginx.Reload(ctx, o.Exec))

	// 2. Remove PHP-FPM pools from every version and reload what changed.
	for _, ver := range o.phpVersions() {
		path := filepath.Join(fmt.Sprintf(o.Paths.PHPPoolDir, ver), "zx-"+name+".conf")
		if err := os.Remove(path); err == nil {
			res, err := o.Exec.Run(ctx, binSystemctl, []string{"reload", "php" + ver + "-fpm"}, time.Minute)
			note("php "+ver, execErr(res, err))
		}
	}

	// 3. Drop the database and its user.
	sql := fmt.Sprintf("DROP DATABASE IF EXISTS `%[1]s`;\nDROP USER IF EXISTS '%[2]s'@'localhost';\nFLUSH PRIVILEGES;\n", db, dbUser)
	res, err := o.Exec.RunInput(ctx, binMariadb, []string{"--protocol=socket", "-uroot"}, sql, time.Minute)
	note("database", execErr(res, err))

	// 4. Remove files. Only the site's own home directory can be removed.
	home := o.homeDir(name)
	if !strings.HasPrefix(home, filepath.Clean(o.Paths.WebRoot)+string(filepath.Separator)+"zx_") {
		return errors.New("refusing to remove an unexpected path")
	}
	note("files", os.RemoveAll(home))

	// 5. Remove logs.
	if logs, _ := filepath.Glob(filepath.Join(o.Paths.LogDir, "zx-"+name+".*")); len(logs) > 0 {
		for _, l := range logs {
			_ = os.Remove(l)
		}
	}

	// 6. Remove the system account. Exit code 6 means it was already gone.
	res, err = o.Exec.Run(ctx, binUserdel, []string{name}, time.Minute)
	if err == nil && res.ExitCode != 0 && res.ExitCode != 6 {
		note("account", execErr(res, nil))
	} else if err != nil {
		note("account", err)
	}

	if len(problems) > 0 {
		return fmt.Errorf("purge incomplete (retry to finish): %s", strings.Join(problems, "; "))
	}
	return nil
}

// execErr turns a command result into an error, including a non-zero exit code.
func execErr(res executor.Result, err error) error {
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return errors.New(trim(stderrOr(res, nil)))
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
