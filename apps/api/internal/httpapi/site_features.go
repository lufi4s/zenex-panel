package httpapi

import (
	"net/http"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// toggleRequest is the body of the on/off site endpoints. Enabled is a pointer so
// a missing field is refused instead of silently meaning "off".
type toggleRequest struct {
	Enabled *bool `json:"enabled"`
}

// toggleAction names the audit entry for an on/off change, for example
// "site.maintenance.on".
func toggleAction(base string, enabled bool) string {
	if enabled {
		return base + ".on"
	}
	return base + ".off"
}

// handleSetAutoUpdate turns WordPress auto-updates on or off for a website.
func (d Deps) handleSetAutoUpdate(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	var in toggleRequest
	if err := decodeJSON(r, &in); err != nil || in.Enabled == nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if err := d.Sites.SetSiteAutoUpdate(r.Context(), site.ID, *in.Enabled); err != nil {
		d.internal(w, r, "sites.auto_update", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: toggleAction("site.auto_update", *in.Enabled), TargetType: "site", TargetID: site.ID, Result: "success"})
	updated, _ := d.Sites.GetSite(r.Context(), site.ID)
	writeJSON(w, http.StatusOK, updated)
}

// handleSetMaintenance shows or hides the maintenance page for a website.
func (d Deps) handleSetMaintenance(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	var in toggleRequest
	if err := decodeJSON(r, &in); err != nil || in.Enabled == nil {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	if err := d.Manage.SetMaintenance(r.Context(), site, *in.Enabled); err != nil {
		d.manageError(w, r, "sites.maintenance", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: toggleAction("site.maintenance", *in.Enabled), TargetType: "site", TargetID: site.ID, Result: "success"})
	updated, _ := d.Sites.GetSite(r.Context(), site.ID)
	writeJSON(w, http.StatusOK, updated)
}

// handleListBackups returns a website's backups, newest first.
func (d Deps) handleListBackups(w http.ResponseWriter, r *http.Request) {
	site, _, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	list, err := d.Sites.ListBackups(r.Context(), site.ID)
	if err != nil {
		d.internal(w, r, "sites.backups", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleStartBackup starts a backup job for a ready website.
func (d Deps) handleStartBackup(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	jobID, err := d.Manage.StartBackup(r.Context(), site, user.ID)
	if err != nil {
		d.manageError(w, r, "sites.backup", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.backup", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}
