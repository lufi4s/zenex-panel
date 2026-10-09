package helper

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// runsWithoutLock lists operations that use only their own work folder, so they may run
// beside other operations. Everything else shares web server and PHP configuration.
func runsWithoutLock(op string) bool {
	return op == "cpanel.scan" || op == "cpanel.pull"
}

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
