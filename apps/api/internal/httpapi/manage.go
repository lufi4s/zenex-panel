package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/helperclient"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// SiteManager performs operations on existing sites.
type SiteManager interface {
	Suspend(ctx context.Context, site store.Site) error
	Resume(ctx context.Context, site store.Site) error
	RestartPHP(ctx context.Context, site store.Site) error
	SwitchPHP(ctx context.Context, site store.Site, to string) error
	PHPVersions(ctx context.Context) ([]string, error)
	Logs(ctx context.Context, site store.Site) (string, error)
	Delete(ctx context.Context, site store.Site, actorID string) (string, error)
	Services(ctx context.Context) ([]manage.ServiceState, error)
	Files(ctx context.Context, site store.Site, op, path, content string) (string, error)
}

// manageError maps an operation failure to a response. Refusals carry a message
// meant for the customer; anything else is logged and hidden.
func (d Deps) manageError(w http.ResponseWriter, r *http.Request, op string, err error) {
	var refusal *manage.Refusal
	if errors.As(err, &refusal) {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusConflict, Code: "not_allowed", Message: refusal.Message})
		return
	}
	var helperErr *helperclient.Error
	if errors.As(err, &helperErr) {
		// Helper messages describe the failed system step. They are written so they
		// never contain secrets.
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusUnprocessableEntity, Code: "operation_failed", Message: helperErr.Message})
		return
	}
	d.internal(w, r, op, err)
}

// siteAction runs a simple action on a site the caller can access.
func (d Deps) siteAction(op, action string, run func(ctx context.Context, site store.Site) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, user, ok := d.loadSiteForUser(w, r)
		if !ok {
			return
		}
		if err := run(r.Context(), site); err != nil {
			d.manageError(w, r, op, err)
			return
		}
		d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: action, TargetType: "site", TargetID: site.ID, Result: "success"})
		site, _ = d.Sites.GetSite(r.Context(), site.ID)
		writeJSON(w, http.StatusOK, site)
	}
}

func (d Deps) handleSuspendSite() http.HandlerFunc {
	return d.siteAction("sites.suspend", "site.suspend", func(ctx context.Context, s store.Site) error {
		return d.Manage.Suspend(ctx, s)
	})
}

func (d Deps) handleResumeSite() http.HandlerFunc {
	return d.siteAction("sites.resume", "site.resume", func(ctx context.Context, s store.Site) error {
		return d.Manage.Resume(ctx, s)
	})
}

func (d Deps) handleRestartPHP() http.HandlerFunc {
	return d.siteAction("sites.php_restart", "site.php_restart", func(ctx context.Context, s store.Site) error {
		return d.Manage.RestartPHP(ctx, s)
	})
}

type phpRequest struct {
	Version string `json:"version"`
}

func (d Deps) handleSwitchPHP(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	var in phpRequest
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if err := d.Manage.SwitchPHP(r.Context(), site, in.Version); err != nil {
		d.manageError(w, r, "sites.php_switch", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.php_switch", TargetType: "site", TargetID: site.ID, Result: "success"})
	updated, _ := d.Sites.GetSite(r.Context(), site.ID)
	writeJSON(w, http.StatusOK, updated)
}

func (d Deps) handleSiteLogs(w http.ResponseWriter, r *http.Request) {
	site, _, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	out, err := d.Manage.Logs(r.Context(), site)
	if err != nil {
		d.manageError(w, r, "sites.logs", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"log": out})
}

func (d Deps) handleListPHPVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := d.Manage.PHPVersions(r.Context())
	if err != nil {
		d.manageError(w, r, "php.versions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"versions": versions})
}

func (d Deps) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	jobID, err := d.Manage.Delete(r.Context(), site, user.ID)
	if err != nil {
		d.manageError(w, r, "sites.delete", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.delete", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID, "status": "deleting"})
}
