package helper

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

// Panel update: the API asks for the installed and latest versions, and can start
// an update. The update runs as a transient systemd unit so it survives the API
// and helper restarting during the install.
const (
	panelRepo      = "/opt/zenex/src"
	panelBranch    = "refs/heads/main"
	updateUnit     = "zenex-update"
	updateScript   = panelRepo + "/infrastructure/deployment/update.sh"
	updateLogPath  = "/var/log/zenex/update.log"
	updateLogBytes = 64 << 10
	updateLogLines = 200
)

// updateStatus is the JSON the API receives from panel.update-status.
type updateStatus struct {
	State string `json:"state"` // idle, running, succeeded or failed
	Log   string `json:"log"`
}

// panelVersion returns the short commit the panel is running.
func (o *Ops) panelVersion(ctx context.Context) (Result, error) {
	res, err := o.Exec.Run(ctx, binGit, []string{"-C", panelRepo, "rev-parse", "--short=7", "HEAD"}, 15*time.Second)
	if err := execErr(res, err); err != nil {
		return Result{}, errors.New("could not read the installed panel version")
	}
	return Result{Output: strings.TrimSpace(res.Stdout)}, nil
}

// panelLatest asks GitHub which commit main points to.
func (o *Ops) panelLatest(ctx context.Context) (Result, error) {
	res, err := o.Exec.Run(ctx, binGit, []string{"-C", panelRepo, "ls-remote", "origin", panelBranch}, 30*time.Second)
	if err := execErr(res, err); err != nil {
		return Result{}, errors.New("could not reach GitHub to check for a new version")
	}
	fields := strings.Fields(res.Stdout)
	if len(fields) == 0 || len(fields[0]) < 7 {
		return Result{}, errors.New("GitHub did not report the latest version")
	}
	return Result{Output: fields[0][:7]}, nil
}

// updateRunning reports whether the update unit is currently active.
func (o *Ops) updateRunning(ctx context.Context) bool {
	res, err := o.Exec.Run(ctx, binSystemctl, []string{"is-active", updateUnit}, 15*time.Second)
	if err != nil {
		return false
	}
	state := strings.TrimSpace(res.Stdout)
	return state == "active" || state == "activating" || state == "reloading"
}

// updateStart launches the update script as a transient unit.
func (o *Ops) updateStart(ctx context.Context) (Result, error) {
	if o.updateRunning(ctx) {
		return Result{}, errors.New("an update is already running")
	}
	// Clear a previous failed run so its unit name can be used again.
	_, _ = o.Exec.Run(ctx, binSystemctl, []string{"reset-failed", updateUnit + ".service"}, 15*time.Second)
	res, err := o.Exec.Run(ctx, binSystemdRun, []string{
		"--unit=" + updateUnit,
		"--collect",
		"--description=Zenex panel update",
		binBash, updateScript,
	}, 30*time.Second)
	if err := execErr(res, err); err != nil {
		return Result{}, errors.New("the update could not be started")
	}
	return Result{}, nil
}

// updateStatusOutput returns the state of the last update and the end of its log.
func (o *Ops) updateStatusOutput(ctx context.Context) (Result, error) {
	log := readLogTail(updateLogPath)
	body, err := json.Marshal(updateStatus{
		State: updateState(log, o.updateRunning(ctx)),
		Log:   log,
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Output: string(body)}, nil
}

// updateState decides what the panel shows. While the unit runs the update is
// running. Otherwise the last "== update" line in the log says how it ended.
func updateState(log string, running bool) string {
	if running {
		return "running"
	}
	last := ""
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, "== update") {
			last = line
		}
	}
	switch {
	case last == "":
		return "idle"
	case strings.Contains(last, "finished OK"):
		return "succeeded"
	default:
		// "FAILED", or "started" with no end: the update stopped early.
		return "failed"
	}
}

// readLogTail returns the last lines of the update log, capped in size.
func readLogTail(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > updateLogBytes {
		data = data[len(data)-updateLogBytes:]
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > updateLogLines {
		lines = lines[len(lines)-updateLogLines:]
	}
	return strings.Join(lines, "\n")
}
