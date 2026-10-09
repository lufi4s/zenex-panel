package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
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
	busy, err := d.siteBusyWithBackupWork(r.Context(), site.ID)
	if err != nil {
		d.internal(w, r, "sites.backup.busy", err)
		return
	}
	if busy {
		writeError(w, requestIDFrom(r), errBackupBusy)
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

// errBackupBusy is returned when a backup or restore is already queued or running for a website.
var errBackupBusy = APIError{Status: http.StatusConflict, Code: "backup_in_progress", Message: "A backup or restore is already running for this website. Wait for it to finish."}

// siteBusyWithBackupWork reports whether the website's latest backup or restore job is
// queued or running. Only one such job runs per website, so the latest one decides.
func (d Deps) siteBusyWithBackupWork(ctx context.Context, siteID string) (bool, error) {
	for _, jobType := range []string{manage.JobBackup, manage.JobRestore, manage.JobMigrate} {
		job, err := d.Sites.LatestJobForSiteOfType(ctx, siteID, jobType)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return false, err
		}
		if job.Status == "queued" || job.Status == "running" {
			return true, nil
		}
	}
	return false, nil
}

// handleRestoreBackup replaces a website's files and database with one of its backups.
// A copy of the current website is taken first, as a job step.
func (d Deps) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	backupID, err := strconv.ParseInt(r.PathValue("backup_id"), 10, 64)
	if err != nil || backupID < 1 {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return
	}
	backup, err := d.Sites.GetBackup(r.Context(), site.ID, backupID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return
	}
	if err != nil {
		d.internal(w, r, "sites.restore.backup", err)
		return
	}
	busy, err := d.siteBusyWithBackupWork(r.Context(), site.ID)
	if err != nil {
		d.internal(w, r, "sites.restore.busy", err)
		return
	}
	if busy {
		writeError(w, requestIDFrom(r), errBackupBusy)
		return
	}
	jobID, err := d.Manage.StartRestore(r.Context(), site, backup, site.Domain, user.ID)
	if err != nil {
		d.manageError(w, r, "sites.restore", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.restore", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// backupAllResult is the answer of POST /api/v1/sites/backup-all.
type backupAllResult struct {
	Started int `json:"started"`
	Skipped int `json:"skipped"`
}

// handleBackupAll starts a backup of every ready website the customer owns (every website
// for an administrator). Websites with a backup or restore already running are skipped.
// The jobs run in the background like POST /api/v1/sites/{id}/backup.
func (d Deps) handleBackupAll(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	if !runNowMu.TryLock() {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusConflict, Code: "backup_run_in_progress", Message: "A backup run is already starting. Wait a moment and try again."})
		return
	}
	defer runNowMu.Unlock()

	owner := user.ID
	if isAdmin(user) {
		owner = ""
	}
	sites, err := d.Sites.ListSites(r.Context(), owner)
	if err != nil {
		d.internal(w, r, "sites.backup_all.list", err)
		return
	}
	busy, err := d.Sites.SitesWithActiveBackup(r.Context())
	if err != nil {
		d.internal(w, r, "sites.backup_all.active", err)
		return
	}
	active := make(map[string]bool, len(busy))
	for _, id := range busy {
		active[id] = true
	}

	var res backupAllResult
	for _, site := range sites {
		if site.State != "ready" || active[site.ID] {
			res.Skipped++
			continue
		}
		if _, err := d.Manage.StartBackup(r.Context(), site, user.ID); err != nil {
			d.Log.Warn("backup-all: backup not started", "site_id", site.ID, "error", err)
			res.Skipped++
			continue
		}
		active[site.ID] = true
		res.Started++
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.backup_all", TargetType: "user", TargetID: user.ID, Result: "success"})
	writeJSON(w, http.StatusAccepted, res)
}

// backupProgressStep is one step of a backup job as the progress bar shows it.
type backupProgressStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// backupProgress is the latest backup job of a website. Percent counts a succeeded
// step as a whole step and a running step as half of one.
type backupProgress struct {
	JobID   string               `json:"job_id"`
	Status  string               `json:"status"`
	Percent int                  `json:"percent"`
	Steps   []backupProgressStep `json:"steps"`
}

// handleBackupProgress returns the progress of the website's latest backup job. A
// website without any backup job gets 200 with an empty job_id.
func (d Deps) handleBackupProgress(w http.ResponseWriter, r *http.Request) {
	site, _, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	job, err := d.Sites.LatestJobForSiteOfType(r.Context(), site.ID, manage.JobBackup)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]string{"job_id": ""})
		return
	}
	if err != nil {
		d.internal(w, r, "sites.backup_progress", err)
		return
	}
	steps, err := d.Sites.JobSteps(r.Context(), job.ID)
	if err != nil {
		d.internal(w, r, "sites.backup_progress", err)
		return
	}
	out := backupProgress{JobID: job.ID, Status: job.Status, Steps: make([]backupProgressStep, 0, len(steps))}
	statuses := make([]string, 0, len(steps))
	for _, st := range steps {
		out.Steps = append(out.Steps, backupProgressStep{Name: st.Name, Status: st.Status})
		statuses = append(statuses, st.Status)
	}
	out.Percent = manage.ProgressPercent(statuses)
	writeJSON(w, http.StatusOK, out)
}
