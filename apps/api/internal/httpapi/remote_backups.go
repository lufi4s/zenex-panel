package httpapi

import (
	"net/http"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/manage"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// handleListRemoteBackups lists the Zenex backups on the saved SFTP server. Any panel that
// uses the same server sees them, so this is how a new panel finds a website's backups.
func (d Deps) handleListRemoteBackups(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.requireAdminUser(w, r); !ok {
		return
	}
	allowSlowRequest(w, 0, 6*time.Minute)
	list, err := d.Manage.ListRemoteBackups(r.Context())
	if err != nil {
		d.manageError(w, r, "settings.backups.remote", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// restoreRemoteRequest names the backup to restore, as the listing returned it.
type restoreRemoteRequest struct {
	Path string `json:"path"`
}

// handleRestoreRemote restores a website from a backup on the SFTP server. The backup must
// be in the current listing, so a path typed by hand cannot reach other files on the server.
func (d Deps) handleRestoreRemote(w http.ResponseWriter, r *http.Request) {
	site, user, ok := d.loadSiteForUser(w, r)
	if !ok {
		return
	}
	if !isAdmin(user) {
		writeError(w, requestIDFrom(r), APIError{Status: http.StatusForbidden, Code: "forbidden", Message: "Only an administrator can restore from the backup server."})
		return
	}
	var in restoreRemoteRequest
	if err := decodeJSON(r, &in); err != nil || in.Path == "" {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	busy, err := d.siteBusyWithBackupWork(r.Context(), site.ID)
	if err != nil {
		d.internal(w, r, "sites.restore_remote.busy", err)
		return
	}
	if busy {
		writeError(w, requestIDFrom(r), errBackupBusy)
		return
	}
	allowSlowRequest(w, 0, 6*time.Minute)
	list, err := d.Manage.ListRemoteBackups(r.Context())
	if err != nil {
		d.manageError(w, r, "sites.restore_remote.list", err)
		return
	}
	var found *manage.RemoteBackup
	for i := range list {
		if list[i].Path == in.Path {
			found = &list[i]
			break
		}
	}
	if found == nil {
		writeError(w, requestIDFrom(r), ErrNotFound)
		return
	}
	jobID, err := d.Manage.StartRemoteRestore(r.Context(), site, *found, user.ID)
	if err != nil {
		d.manageError(w, r, "sites.restore_remote", err)
		return
	}
	d.audit(r.Context(), r, store.AuditEntry{ActorUserID: user.ID, ActorRole: primaryRole(user), Action: "site.restore_remote", TargetType: "site", TargetID: site.ID, Result: "success"})
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}
