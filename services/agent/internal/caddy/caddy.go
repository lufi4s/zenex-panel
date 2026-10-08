// Package caddy contains the only web-server operations the Zenex helper may
// perform. Caddy obtains and renews HTTPS certificates by itself, so the helper
// only writes site files and reloads.
package caddy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zenexcloud/zenex-panel/services/agent/internal/executor"
)

const (
	binCaddy     = "/usr/bin/caddy"
	binSystemctl = "/usr/bin/systemctl"
	// MainConfig imports every enabled site file.
	MainConfig   = "/etc/caddy/Caddyfile"
	validateTime = 30 * time.Second
	reloadTime   = 60 * time.Second
)

// AllowedBinaries lists every program this package may execute.
var AllowedBinaries = []string{binCaddy, binSystemctl}

var ErrConfigInvalid = errors.New("caddy rejected the configuration; site not changed")

// Validate checks the whole configuration, including every imported site file.
func Validate(ctx context.Context, ex executor.Executor) error {
	res, err := ex.Run(ctx, binCaddy, []string{"validate", "--config", MainConfig, "--adapter", "caddyfile"}, validateTime)
	if err != nil {
		return fmt.Errorf("caddy validate: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%w: %s", ErrConfigInvalid, res.Stderr)
	}
	return nil
}

// Reload validates the configuration and reloads only if validation passes.
func Reload(ctx context.Context, ex executor.Executor) error {
	if err := Validate(ctx, ex); err != nil {
		return err
	}
	res, err := ex.Run(ctx, binSystemctl, []string{"reload", "caddy"}, reloadTime)
	if err != nil {
		return fmt.Errorf("reload caddy: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("reload caddy exited %d: %s", res.ExitCode, res.Stderr)
	}
	return nil
}
