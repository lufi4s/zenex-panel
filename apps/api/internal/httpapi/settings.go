package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/alerts"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/helperclient"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// AlertManager reads and saves the alert configuration. *alerts.Service satisfies it.
type AlertManager interface {
	View(ctx context.Context) (alerts.View, error)
	Update(ctx context.Context, userID string, in alerts.Input) error
	SendTestEmail(ctx context.Context) error
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

var (
	sftpLabelRe    = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	sftpUsernameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
)

const maxSFTPPathLen = 200

// validBackupSettings checks the hour (0-23), retention (1-90 days) and the destination.
// It returns a trimmed copy, with the SFTP folder cleaned.
func validBackupSettings(in store.BackupSettings) (store.BackupSettings, string) {
	if in.ScheduleHour < 0 || in.ScheduleHour > 23 {
		return in, "The backup hour must be between 0 and 23."
	}
	if in.RetentionDays < 1 || in.RetentionDays > 90 {
		return in, "Keep backups for between 1 and 90 days."
	}
	dest := &in.Destination
	dest.Type = strings.ToLower(strings.TrimSpace(dest.Type))
	if dest.Type == "" {
		dest.Type = store.BackupDestLocal
	}
	switch dest.Type {
	case store.BackupDestLocal:
		return in, ""
	case store.BackupDestSFTP:
		s := &dest.SFTP
		s.Host = strings.TrimSpace(s.Host)
		s.Username = strings.TrimSpace(s.Username)
		s.Path = strings.TrimSpace(s.Path)
		if !validSFTPHost(s.Host) {
			return in, "Enter the SFTP server as a host name or IP address, without spaces or slashes."
		}
		if s.Port < 1 || s.Port > 65535 {
			return in, "The SFTP port must be between 1 and 65535."
		}
		if !sftpUsernameRe.MatchString(s.Username) {
			return in, "The SFTP username may use lowercase letters, digits, _ and -, and must start with a letter or _ (32 characters at most)."
		}
		if !validSFTPPath(s.Path) {
			return in, "The SFTP folder must be an absolute path below / of at most 200 characters, without \"..\"."
		}
		s.Path = path.Clean(s.Path)
		return in, ""
	}
	return in, "Choose local or sftp as the backup destination."
}

// validSFTPHost accepts an IPv4/IPv6 address or a dotted host name.
func validSFTPHost(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if !sftpLabelRe.MatchString(label) {
			return false
		}
	}
	return true
}

// validSFTPPath requires an absolute folder other than the root, with no ".." and no control characters.
func validSFTPPath(p string) bool {
	if strings.TrimSpace(p) == "" || len(p) > maxSFTPPathLen || !strings.HasPrefix(p, "/") || path.Clean(p) == "/" {
		return false
	}
	if strings.Contains(p, "..") {
		return false
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// handleGetSFTPKey returns the backup public key, or 404 when no key pair was created yet.
func (d Deps) handleGetSFTPKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.requireAdminUser(w, r); !ok {
		return
	}
	key, err := d.Sites.GetSFTPPublicKey(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusNotFound, Code: "key_missing", Message: "No backup key exists yet. Create one first."})
		return
	}
	if err != nil {
		d.internal(w, r, "settings.backups.sftp_key.get", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": key})
}

// handleCreateSFTPKey creates the backup key pair if the helper does not have one yet.
func (d Deps) handleCreateSFTPKey(w http.ResponseWriter, r *http.Request) {
	user, ok := d.requireAdminUser(w, r)
	if !ok {
		return
	}
	key, err := d.Manage.SFTPPublicKey(r.Context())
	if err != nil {
		d.manageError(w, r, "settings.backups.sftp_key", err)
		return
	}
	if err := d.Sites.SetSFTPPublicKey(r.Context(), user.ID, key); err != nil {
		d.internal(w, r, "settings.backups.sftp_key.set", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "settings.backups.sftp_key", TargetType: "settings", TargetID: "backup_sftp_key", Result: "success"})
	writeJSON(w, http.StatusOK, map[string]string{"public_key": key})
}

// handleTestSFTP checks the saved SFTP destination. Failures are returned as 502 with a short message.
func (d Deps) handleTestSFTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.requireAdminUser(w, r); !ok {
		return
	}
	settings, err := d.Sites.GetBackupSettings(r.Context())
	if err != nil {
		d.internal(w, r, "settings.backups.sftp_test.settings", err)
		return
	}
	if settings.Destination.Type != store.BackupDestSFTP {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadRequest, Code: "sftp_not_configured", Message: "Save an SFTP destination before testing it."})
		return
	}
	if err := d.Manage.TestSFTP(r.Context(), settings.Destination.SFTP); err != nil {
		msg := "the SFTP server could not be reached with the saved settings; check the host, port, username and folder, and that the panel key is installed"
		var helperErr *helperclient.Error
		if errors.As(err, &helperErr) && helperErr.Message != "" {
			msg = helperErr.Message
		}
		if runes := []rune(msg); len(runes) > 200 {
			msg = string(runes[:200])
		}
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadGateway, Code: "sftp_failed", Message: msg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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

// handleTestAlertEmail sends one test message with the saved email settings.
// Delivery failures return a short message without the password or SMTP transcript.
func (d Deps) handleTestAlertEmail(w http.ResponseWriter, r *http.Request) {
	user, ok := d.requireAdminUser(w, r)
	if !ok {
		return
	}
	audit := func(result, code string) {
		d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "alerts.test_email", TargetType: "settings", TargetID: "alert_settings", Result: result, ErrorCode: code})
	}
	err := d.Alerts.SendTestEmail(r.Context())
	var delivery *alerts.DeliveryError
	switch {
	case errors.Is(err, alerts.ErrEmailNotConfigured):
		audit("failure", "email_not_configured")
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadRequest, Code: "email_not_configured", Message: "Save a complete email configuration (server, sender, recipient and password) and turn email alerts on first."})
		return
	case errors.Is(err, alerts.ErrNoSecretKey):
		audit("failure", ErrSecretKeyMissing.Code)
		writeError(w, requestIDFrom(r), ErrSecretKeyMissing)
		return
	case errors.As(err, &delivery):
		audit("failure", "email_failed")
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadGateway, Code: "email_failed", Message: delivery.Message})
		return
	case err != nil:
		audit("failure", ErrInternal.Code)
		d.internal(w, r, "settings.alerts.test_email", err)
		return
	}
	audit("success", "")
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}
