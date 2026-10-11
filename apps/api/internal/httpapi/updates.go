package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/update"
)

// UpdateService checks for and starts panel updates.
type UpdateService interface {
	Status(ctx context.Context) (update.Status, error)
	Start(ctx context.Context) error
}

// handleGetUpdate reports the installed and latest panel versions. Administrators only.
func (d Deps) handleGetUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	if !isAdmin(user) {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "Only administrators can see panel updates."})
		return
	}
	st, err := d.Updates.Status(r.Context())
	if err != nil {
		d.manageError(w, r, "update.status", err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleStartUpdate starts an update to the latest version. Administrators only.
// The panel restarts during the update, so the browser may lose its connection for
// a minute; the Settings page reconnects by itself.
func (d Deps) handleStartUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	if !isAdmin(user) {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "Only administrators can update the panel."})
		return
	}
	err := d.Updates.Start(r.Context())
	outcome := "success"
	if err != nil {
		outcome = "failure"
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "panel.update", TargetType: "settings", TargetID: "panel", Result: outcome})

	switch {
	case errors.Is(err, update.ErrRunning):
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusConflict, Code: "update_running", Message: "An update is already running. Wait for it to finish."})
	case err != nil:
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusBadGateway, Code: "update_failed", Message: err.Error()})
	default:
		writeJSON(w, http.StatusAccepted, map[string]string{"state": "running"})
	}
}
