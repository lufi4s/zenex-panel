package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/alerts"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// AlertManager reads and saves the alert configuration. *alerts.Service satisfies it.
type AlertManager interface {
	View(ctx context.Context) (alerts.View, error)
	Update(ctx context.Context, userID string, in alerts.Input) error
}

var (
	ErrPHPNotInstalled  = APIError{Status: http.StatusBadRequest, Code: "php_not_installed", Message: "choose a PHP version that is installed on this server"}
	ErrSecretKeyMissing = APIError{Status: http.StatusConflict, Code: "secret_key_missing", Message: "the server secret is not configured, so passwords and tokens cannot be saved"}
)

// requireAdminUser returns the signed-in user when they are an administrator.
func (d Deps) requireAdminUser(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	user, _ := currentUser(r)
	if !isAdmin(user) {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "Only administrators can do this."})
		return nil, false
	}
	return user, true
}

// defaultPHPVersion is the PHP version new websites use: the saved default, or
// the server's configured version when no default has been saved.
func (d Deps) defaultPHPVersion(ctx context.Context) (string, error) {
	saved, err := d.Sites.GetSiteDefaults(ctx)
	if err != nil {
		return "", err
	}
	if saved.PHPVersion != "" {
		return saved.PHPVersion, nil
	}
	return d.Site.PHPVersion, nil
}

// ---------------------------------------------------------------------------
// Site defaults
// ---------------------------------------------------------------------------

func (d Deps) handleGetSiteDefaults(w http.ResponseWriter, r *http.Request) {
	version, err := d.defaultPHPVersion(r.Context())
	if err != nil {
		d.internal(w, r, "settings.defaults.get", err)
		return
	}
	writeJSON(w, http.StatusOK, store.SiteDefaults{PHPVersion: version})
}

// handlePutSiteDefaults saves the PHP version new websites start with. The
// version must be one the helper reports as installed.
func (d Deps) handlePutSiteDefaults(w http.ResponseWriter, r *http.Request) {
	user, ok := d.requireAdminUser(w, r)
	if !ok {
		return
	}
	var in store.SiteDefaults
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	version := strings.TrimSpace(in.PHPVersion)
	installed, err := d.Manage.PHPVersions(r.Context())
	if err != nil {
		d.manageError(w, r, "settings.defaults.php", err)
		return
	}
	if !slices.Contains(installed, version) {
		writeError(w, requestIDFrom(r), ErrPHPNotInstalled)
		return
	}
	if err := d.Sites.SetSiteDefaults(r.Context(), user.ID, store.SiteDefaults{PHPVersion: version}); err != nil {
		d.internal(w, r, "settings.defaults.set", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "settings.defaults.update", TargetType: "settings", TargetID: "site_defaults", Result: "success"})
	writeJSON(w, http.StatusOK, store.SiteDefaults{PHPVersion: version})
}

// ---------------------------------------------------------------------------
// Backup schedule and retention
// ---------------------------------------------------------------------------

// validBackupSettings checks the hour (0-23) and retention (1-90 days).
func validBackupSettings(in store.BackupSettings) (store.BackupSettings, string) {
	if in.ScheduleHour < 0 || in.ScheduleHour > 23 {
		return in, "The backup hour must be between 0 and 23."
	}
	if in.RetentionDays < 1 || in.RetentionDays > 90 {
		return in, "Keep backups for between 1 and 90 days."
	}
	return in, ""
}

func (d Deps) handleGetBackupSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.requireAdminUser(w, r); !ok {
		return
	}
	cfg, err := d.Sites.GetBackupSettings(r.Context())
	if err != nil {
		d.internal(w, r, "settings.backups.get", err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (d Deps) handlePutBackupSettings(w http.ResponseWriter, r *http.Request) {
	user, ok := d.requireAdminUser(w, r)
	if !ok {
		return
	}
	var in store.BackupSettings
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	cfg, problem := validBackupSettings(in)
	if problem != "" {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadRequest, Code: "invalid_backup_settings", Message: problem})
		return
	}
	if err := d.Sites.SetBackupSettings(r.Context(), user.ID, cfg); err != nil {
		d.internal(w, r, "settings.backups.set", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "settings.backups.update", TargetType: "settings", TargetID: "backup_settings", Result: "success"})
	writeJSON(w, http.StatusOK, cfg)
}

// ---------------------------------------------------------------------------
// Alerts
// ---------------------------------------------------------------------------

func (d Deps) handleGetAlerts(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.requireAdminUser(w, r); !ok {
		return
	}
	view, err := d.Alerts.View(r.Context())
	if err != nil {
		d.internal(w, r, "settings.alerts.get", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handlePutAlerts saves the alert configuration. Secrets are never echoed back.
func (d Deps) handlePutAlerts(w http.ResponseWriter, r *http.Request) {
	user, ok := d.requireAdminUser(w, r)
	if !ok {
		return
	}
	var in alerts.Input
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	err := d.Alerts.Update(r.Context(), user.ID, in)
	var verr *alerts.ValidationError
	switch {
	case errors.As(err, &verr):
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadRequest, Code: "invalid_alerts", Message: verr.Message})
		return
	case errors.Is(err, alerts.ErrNoSecretKey):
		writeError(w, requestIDFrom(r), ErrSecretKeyMissing)
		return
	case err != nil:
		d.internal(w, r, "settings.alerts.set", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "settings.alerts.update", TargetType: "settings", TargetID: "alert_settings", Result: "success"})
	view, err := d.Alerts.View(r.Context())
	if err != nil {
		d.internal(w, r, "settings.alerts.get", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
