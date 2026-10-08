// Package manage performs day-to-day operations on existing websites: suspend,
// resume, PHP restart and version change, logs, and permanent deletion.
//
// Every operation checks the site's current state first, so an action that does
// not make sense (for example, suspending a site that is still being built) is
// refused with a clear reason instead of being attempted.
package manage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// JobDelete is the job type recorded for site deletion.
const JobDelete = "site.delete"

// Refusal is returned when an action is not allowed in the site's current state.
// Its message is shown to the customer as is.
type Refusal struct{ Message string }

func (e *Refusal) Error() string { return e.Message }

func refuse(format string, args ...any) error {
	return &Refusal{Message: fmt.Sprintf(format, args...)}
}

// Store is the persistence surface the manager needs.
type Store interface {
	SetSiteState(ctx context.Context, siteID, state string) error
	SetSitePHPVersion(ctx context.Context, siteID, version string) error
	MarkSiteDeleted(ctx context.Context, siteID string) error
	CreateManagementJob(ctx context.Context, actorID, siteID, nodeID, jobType string) (string, error)
	FinishJob(ctx context.Context, jobID, status, errMsg string) error
	AppendJobLog(ctx context.Context, jobID, level, msg string) error
}

// Helper runs privileged operations.
type Helper interface {
	Do(ctx context.Context, op string, args map[string]string) (string, error)
	Output(ctx context.Context, op string, args map[string]string) (string, error)
}

// Manager runs site operations.
type Manager struct {
	Store  Store
	Helper Helper
	Log    *slog.Logger
	// DeleteTimeout bounds the background deletion.
	DeleteTimeout time.Duration
}

func New(s Store, h Helper, log *slog.Logger) *Manager {
	return &Manager{Store: s, Helper: h, Log: log, DeleteTimeout: 10 * time.Minute}
}

func dbNames(site store.Site) map[string]string {
	return map[string]string{"user": site.LinuxUser, "db": site.DBName, "dbuser": site.DBUser}
}

// Suspend takes a live site offline. Files and data are kept.
func (m *Manager) Suspend(ctx context.Context, site store.Site) error {
	if site.State != "ready" {
		return refuse("only a ready website can be suspended (it is %s)", site.State)
	}
	if _, err := m.Helper.Do(ctx, "vhost.disable", map[string]string{"user": site.LinuxUser}); err != nil {
		return err
	}
	return m.Store.SetSiteState(ctx, site.ID, "suspended")
}

// Resume brings a suspended site back online.
func (m *Manager) Resume(ctx context.Context, site store.Site) error {
	if site.State != "suspended" {
		return refuse("only a suspended website can be resumed (it is %s)", site.State)
	}
	if _, err := m.Helper.Do(ctx, "vhost.enable", map[string]string{"user": site.LinuxUser}); err != nil {
		return err
	}
	return m.Store.SetSiteState(ctx, site.ID, "ready")
}

// RestartPHP restarts the PHP service for the site's version. Other sites on the
// same version are briefly affected too.
func (m *Manager) RestartPHP(ctx context.Context, site store.Site) error {
	if site.State != "ready" {
		return refuse("PHP can only be restarted for a ready website (it is %s)", site.State)
	}
	_, err := m.Helper.Do(ctx, "php.restart", map[string]string{"php": site.PHPVersion})
	return err
}

// SwitchPHP moves the site to another installed PHP version.
func (m *Manager) SwitchPHP(ctx context.Context, site store.Site, to string) error {
	if site.State != "ready" {
		return refuse("the PHP version can only be changed for a ready website (it is %s)", site.State)
	}
	if to == site.PHPVersion {
		return refuse("the website already uses PHP %s", to)
	}
	args := map[string]string{"user": site.LinuxUser, "from": site.PHPVersion, "to": to}
	if _, err := m.Helper.Do(ctx, "php.switch", args); err != nil {
		return err
	}
	return m.Store.SetSitePHPVersion(ctx, site.ID, to)
}

// PHPVersions lists the PHP versions installed on the server.
func (m *Manager) PHPVersions(ctx context.Context) ([]string, error) {
	out, err := m.Helper.Output(ctx, "php.versions", map[string]string{})
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, ","), nil
}

// Logs returns the end of the site's error log.
func (m *Manager) Logs(ctx context.Context, site store.Site) (string, error) {
	return m.Helper.Output(ctx, "logs.tail", map[string]string{"user": site.LinuxUser})
}

// Delete permanently removes a website: its files, database, database user,
// nginx and PHP configuration, and system account. It runs in the background as a
// tracked job and returns the job ID. A failed deletion can be retried.
func (m *Manager) Delete(ctx context.Context, site store.Site, actorID string) (string, error) {
	switch site.State {
	case "provisioning":
		return "", refuse("this website is still being built; wait until it finishes")
	case "deleting":
		return "", refuse("this website is already being deleted")
	case "deleted":
		return "", refuse("this website was already deleted")
	}
	if err := m.Store.SetSiteState(ctx, site.ID, "deleting"); err != nil {
		return "", err
	}
	jobID, err := m.Store.CreateManagementJob(ctx, actorID, site.ID, site.NodeID, JobDelete)
	if err != nil {
		return "", err
	}
	go m.runDelete(site, jobID)
	return jobID, nil
}

func (m *Manager) runDelete(site store.Site, jobID string) {
	ctx, cancel := context.WithTimeout(context.Background(), m.DeleteTimeout)
	defer cancel()

	_ = m.Store.AppendJobLog(ctx, jobID, "info", "removing files, database and configuration")
	_, err := m.Helper.Do(ctx, "site.purge", dbNames(site))
	if err != nil {
		msg := err.Error()
		m.Log.Warn("site deletion incomplete", "site_id", site.ID, "error", msg)
		_ = m.Store.AppendJobLog(ctx, jobID, "error", msg)
		_ = m.Store.FinishJob(ctx, jobID, "failed", msg)
		_ = m.Store.SetSiteState(ctx, site.ID, "failed")
		return
	}
	if err := m.Store.MarkSiteDeleted(ctx, site.ID); err != nil {
		_ = m.Store.FinishJob(ctx, jobID, "failed", errors.New("site removed but record update failed: "+err.Error()).Error())
		return
	}
	_ = m.Store.AppendJobLog(ctx, jobID, "info", "website deleted")
	_ = m.Store.FinishJob(ctx, jobID, "succeeded", "")
}
