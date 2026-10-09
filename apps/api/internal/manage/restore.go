package manage

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// JobRestore is the job type recorded for a website restore.
const JobRestore = "site.restore"

// Restore job steps, in execution order. The fetch step exists only for archives that
// are stored on the SFTP server.
const (
	StepSafety  = "save_current"
	StepFetch   = "download"
	StepRestore = "restore"
)

// RestoreSteps returns the ordered step names of a restore job.
func RestoreSteps(remote bool) []string {
	if remote {
		return []string{StepSafety, StepFetch, StepRestore}
	}
	return []string{StepSafety, StepRestore}
}

// RemoteBackup is a Zenex backup found on the SFTP server. Path is the remote file path.
// Domain is the website the backup was taken from.
type RemoteBackup struct {
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	Domain    string    `json:"domain"`
	SiteID    string    `json:"site_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ListRemoteBackups lists the Zenex backups on the saved SFTP server, newest first. Only
// archives with a manifest are listed. Older archives have none and are not offered.
func (m *Manager) ListRemoteBackups(ctx context.Context) ([]RemoteBackup, error) {
	settings, err := m.Store.GetBackupSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings.Destination.Type != store.BackupDestSFTP {
		return nil, refuse("the backups are stored on this server; connect an SFTP backup server to list them")
	}
	dest := settings.Destination.SFTP
	out, err := m.runSFTPOutput(ctx, "backup.discover", dest, sftpArgs(dest, dest.Path))
	if err != nil {
		return nil, err
	}
	var raw []struct {
		File      string `json:"file"`
		SizeBytes int64  `json:"size_bytes"`
		Domain    string `json:"domain"`
		SiteID    string `json:"site_id"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, errors.New("the backup server listing could not be read")
	}
	list := make([]RemoteBackup, 0, len(raw))
	for _, r := range raw {
		created, err := time.Parse(time.RFC3339, r.CreatedAt)
		if err != nil {
			continue
		}
		list = append(list, RemoteBackup{Path: r.File, SizeBytes: r.SizeBytes, Domain: r.Domain, SiteID: r.SiteID, CreatedAt: created})
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

// StartRemoteRestore restores a website from a backup found on the SFTP server, which may
// have been taken on another panel. See StartRestore.
func (m *Manager) StartRemoteRestore(ctx context.Context, site store.Site, rb RemoteBackup, actorID string) (string, error) {
	settings, err := m.Store.GetBackupSettings(ctx)
	if err != nil {
		return "", err
	}
	if settings.Destination.Type != store.BackupDestSFTP {
		return "", refuse("the backups are stored on this server; connect an SFTP backup server first")
	}
	backup := store.Backup{
		SiteID: site.ID, SizeBytes: rb.SizeBytes, CreatedAt: rb.CreatedAt,
		Path: remoteBackupPath(settings.Destination.SFTP.Host, rb.Path),
	}
	return m.StartRestore(ctx, site, backup, rb.Domain, actorID)
}

// StartRestore replaces a website's files and database with the contents of a backup and
// returns the job ID. A copy of the current website is archived first, so the restore can
// be undone. sourceDomain is the domain the backup was taken from; when it differs from the
// website's domain, the stored addresses are changed. The caller must make sure no backup or
// restore is already running for the site.
func (m *Manager) StartRestore(ctx context.Context, site store.Site, backup store.Backup, sourceDomain string, actorID string) (string, error) {
	if site.State != "ready" {
		return "", refuse("only a ready website can be restored (it is %s)", site.State)
	}
	if m.DBPassword == nil {
		return "", errors.New("the database password cannot be derived on this server")
	}
	_, _, remote, err := parseRemoteBackupPath(backup.Path)
	if err != nil {
		return "", err
	}
	jobID, err := m.Store.CreateManagementJob(ctx, actorID, site.ID, site.NodeID, JobRestore)
	if err != nil {
		return "", err
	}
	if err := m.Store.EnsureJobSteps(ctx, jobID, RestoreSteps(remote)); err != nil {
		_ = m.Store.FinishJob(ctx, jobID, "failed", "the restore steps could not be recorded")
		return "", err
	}
	go func() {
		rctx, cancel := context.WithTimeout(context.Background(), m.RestoreTimeout)
		defer cancel()
		m.runRestore(rctx, site, backup, sourceDomain, jobID, actorID)
	}()
	return jobID, nil
}

// runRestore runs the restore and records the job outcome. The customer is told when it
// finishes, whichever way it ends.
func (m *Manager) runRestore(ctx context.Context, site store.Site, backup store.Backup, sourceDomain, jobID, actorID string) {
	if err := m.restoreSite(ctx, site, backup, sourceDomain, jobID); err != nil {
		m.Log.Warn("website restore failed", "site_id", site.ID, "error", err)
		_ = m.Store.AppendJobLog(ctx, jobID, "error", err.Error())
		_ = m.Store.FinishJob(ctx, jobID, "failed", err.Error())
		_ = m.Store.Notify(ctx, actorID, "error", "Restore failed for "+site.Domain,
			"The website was not restored. Details are in the job log.")
		return
	}
	_ = m.Store.AppendJobLog(ctx, jobID, "info", "website restored")
	_ = m.Store.FinishJob(ctx, jobID, "succeeded", "")
	_ = m.Store.Notify(ctx, actorID, "success", "Restore complete", site.Domain+" was restored from a backup.")
}

// restoreSite runs the safety, fetch (SFTP only) and restore steps in order. The first
// failing step stops the restore and its error is returned.
func (m *Manager) restoreSite(ctx context.Context, site store.Site, backup store.Backup, sourceDomain, jobID string) error {
	if err := m.runStep(ctx, jobID, StepSafety, func() error {
		_ = m.Store.AppendJobLog(ctx, jobID, "info", "saving a copy of the current website first")
		archive, size, err := m.makeArchive(ctx, jobID, site)
		if err != nil {
			return err
		}
		return m.Store.InsertBackup(ctx, site.ID, size, archive)
	}); err != nil {
		return err
	}

	archive := backup.Path
	host, remoteFile, remote, err := parseRemoteBackupPath(backup.Path)
	if err != nil {
		return err
	}
	if remote {
		local := BackupRoot + "/" + site.LinuxUser + "/restore-" + time.Now().UTC().Format("20060102-150405") + ".tar.gz"
		// The local copy is removed whether or not the restore worked.
		defer m.removeLocalArchive(ctx, jobID, local)
		if err := m.runStep(ctx, jobID, StepFetch, func() error {
			return m.fetchBackup(ctx, jobID, site, host, remoteFile, local)
		}); err != nil {
			return err
		}
		archive = local
	}

	return m.runStep(ctx, jobID, StepRestore, func() error {
		_ = m.Store.AppendJobLog(ctx, jobID, "info", "restoring files and database")
		_, err := m.Helper.Do(ctx, "backup.restore", m.restoreArgs(site, archive, sourceDomain))
		return err
	})
}

// restoreArgs are the arguments of the helper's backup.restore for a website.
func (m *Manager) restoreArgs(site store.Site, archive, sourceDomain string) map[string]string {
	args := map[string]string{
		"user": site.LinuxUser, "database": site.DBName, "docroot": DocRoot(site.LinuxUser), "archive": archive,
		// The site's own database account: the archive's wp-config.php is written again with it.
		"dbpass": m.DBPassword(site.ID),
	}
	if sourceDomain != "" && sourceDomain != site.Domain {
		args["from_domain"] = sourceDomain
		args["to_domain"] = site.Domain
	}
	return args
}

// fetchBackup downloads an archive from the SFTP server the backup settings point to.
// The record stores only the host, so the port and username come from the saved settings.
func (m *Manager) fetchBackup(ctx context.Context, jobID string, site store.Site, host, remoteFile, local string) error {
	settings, err := m.Store.GetBackupSettings(ctx)
	if err != nil {
		return err
	}
	if settings.Destination.Type != store.BackupDestSFTP {
		return errors.New("this backup is on an SFTP server; set up that destination in the backup settings first")
	}
	_ = m.Store.AppendJobLog(ctx, jobID, "info", "downloading the backup from the SFTP server")
	dest := settings.Destination.SFTP
	dest.Host = host
	args := sftpArgs(dest, remoteFile)
	args["file"] = local
	args["user"] = site.LinuxUser
	return m.runSFTPOp(ctx, "backup.download", dest, args)
}
