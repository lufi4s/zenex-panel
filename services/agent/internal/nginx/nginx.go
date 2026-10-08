// Package nginx contains the only Nginx operations the Agent may perform.
// The binary paths and unit name are fixed here; callers cannot supply them.
package nginx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

const (
	binNginx      = "/usr/sbin/nginx"
	binSystemctl  = "/usr/bin/systemctl"
	testTimeout   = 15 * time.Second
	reloadTimeout = 30 * time.Second
)

// AllowedBinaries lists every program this package may execute.
// The Agent passes this to executor.NewRunner at startup.
var AllowedBinaries = []string{binNginx, binSystemctl}

var (
	ErrConfigInvalid = errors.New("nginx -t failed; reload refused")
)

// Test runs `nginx -t`. It returns ErrConfigInvalid (wrapped with the nginx
// output) when the configuration is not valid.
func Test(ctx context.Context, ex executor.Executor) error {
	res, err := ex.Run(ctx, binNginx, []string{"-t"}, testTimeout)
	if err != nil {
		return fmt.Errorf("nginx -t: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%w: %s", ErrConfigInvalid, res.Stderr)
	}
	return nil
}

// Reload validates the configuration first and reloads only if validation
// passes. A failed validation never reaches systemctl.
func Reload(ctx context.Context, ex executor.Executor) error {
	if err := Test(ctx, ex); err != nil {
		return err
	}
	res, err := ex.Run(ctx, binSystemctl, []string{"reload", "nginx"}, reloadTimeout)
	if err != nil {
		return fmt.Errorf("systemctl reload nginx: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("systemctl reload nginx exited %d: %s", res.ExitCode, res.Stderr)
	}
	return nil
}
