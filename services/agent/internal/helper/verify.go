package helper

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// unlockedOps are the operations that do not change the web server or PHP configuration that
// the lock protects. They may run beside other operations. Without this, one backup (up to 20
// minutes) or restore (up to 40) would block every other operation of every website: opening a
// folder, reading a log, building a new site.
//
//   - read-only operations: folder listings, file reads, log tails, service and PHP lists, versions
//   - backup, restore and cPanel copies: they work on one website's files and database, and on
//     the backup folders; the panel never runs two of them for the same website at once
var unlockedOps = map[string]bool{
	"files.list": true, "files.read": true, "logs.tail": true, "services.status": true,
	"php.versions": true, "panel.version": true, "panel.latest": true, "panel.update-status": true,
	"backup.create": true, "backup.restore": true, "backup.upload": true, "backup.download": true,
	"backup.delete": true, "backup.discover": true, "backup.test": true,
	"cpanel.scan": true, "cpanel.pull": true,
}

// runsWithoutLock reports whether an operation skips the lock.
func runsWithoutLock(op string) bool { return unlockedOps[op] }

// wpOutput runs WP-CLI as the site's account and returns what it printed.
func (o *Ops) wpOutput(ctx context.Context, linuxUser string, wpArgs ...string) (string, error) {
	home := o.homeDir(linuxUser)
	argv := []string{"-u", linuxUser, "--", binEnv, "HOME=" + home, binWP, "--path=" + o.docRoot(linuxUser)}
	argv = append(argv, wpArgs...)
	res, err := o.Exec.Run(ctx, binRunuser, argv, 2*time.Minute)
	if err != nil {
		return "", fmt.Errorf("wp-cli: %w", err)
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("wp-cli failed: %s", trim(stderrOr(res, nil)))
	}
	return res.Stdout, nil
}

// wpVerify checks that WordPress runs on a website: it must be installed and able to read its
// database. The reply is the address WordPress has stored for the website.
func (o *Ops) wpVerify(ctx context.Context, args map[string]string) (Result, error) {
	name, err := requireLinuxUser(args)
	if err != nil {
		return Result{}, err
	}
	if _, err := o.wpOutput(ctx, name, "core", "is-installed"); err != nil {
		return Result{}, errors.New("WordPress is not installed or cannot reach its database")
	}
	home, err := o.wpOutput(ctx, name, "option", "get", "home")
	if err != nil {
		return Result{}, errors.New("WordPress could not read its settings")
	}
	return Result{Output: strings.TrimSpace(home)}, nil
}
