package manage

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// JobBackup is the job type recorded for a website backup.
const JobBackup = "site.backup"

const (
	// WebRoot is where the helper keeps each website's account home.
	WebRoot = "/var/www"
	// BackupRoot holds the archives. The helper only writes and deletes below it.
	BackupRoot = "/var/backups/zenex"

	defaultBackupTimeout = 20 * time.Minute
	defaultUpdateTimeout = 15 * time.Minute
)

// FeatureStore is the persistence surface for maintenance, backups and auto-updates.
type FeatureStore interface {
	SetSiteMaintenance(ctx context.Context, siteID string, enabled bool) error
	ListReadySites(ctx context.Context) ([]store.Site, error)
	ListAutoUpdateSites(ctx context.Context) ([]store.Site, error)
	InsertBackup(ctx context.Context, siteID string, sizeBytes int64, path string) error
	BackupsBefore(ctx context.Context, cutoff time.Time) ([]store.Backup, error)
	DeleteBackup(ctx context.Context, id int64) error
	GetBackupSettings(ctx context.Context) (store.BackupSettings, error)
}

// DocRoot is the directory WordPress is installed in for a website.
func DocRoot(linuxUser string) string {
	return WebRoot + "/" + linuxUser + "/htdocs"
}

// SetMaintenance shows or hides the maintenance page. The web server configuration
// is rewritten first, so the stored flag changes only when the change applied.
// The site must be ready: rewriting its configuration would also re-enable a suspended site.
func (m *Manager) SetMaintenance(ctx context.Context, site store.Site, enabled bool) error {
	if site.State != "ready" {
		return refuse("maintenance mode can only be changed for a ready website (it is %s)", site.State)
	}
	flag := "0"
	if enabled {
		flag = "1"
	}
	_, err := m.Helper.Do(ctx, "vhost.write", map[string]string{
		"user": site.LinuxUser, "domain": site.Domain, "maintenance": flag,
	})
	if err != nil {
		return err
	}
	return m.Store.SetSiteMaintenance(ctx, site.ID, enabled)
}

// StartBackup starts a backup in the background and returns the job ID.
func (m *Manager) StartBackup(ctx context.Context, site store.Site, actorID string) (string, error) {
	if site.State != "ready" {
		return "", refuse("only a ready website can be backed up (it is %s)", site.State)
	}
	jobID, err := m.Store.CreateManagementJob(ctx, actorID, site.ID, site.NodeID, JobBackup)
	if err != nil {
		return "", err
	}
	go func() {
		bctx, cancel := context.WithTimeout(context.Background(), m.BackupTimeout)
		defer cancel()
		m.runBackup(bctx, site, jobID, actorID)
	}()
	return jobID, nil
}

// runBackup creates one backup, records the job outcome and tells the customer
// (or the owner, for scheduled runs) when it fails.
func (m *Manager) runBackup(ctx context.Context, site store.Site, jobID, actorID string) {
	notifyID := actorID
	if notifyID == "" {
		notifyID = site.OwnerID
	}
	if err := m.backupSite(ctx, site, jobID); err != nil {
		m.Log.Warn("website backup failed", "site_id", site.ID, "error", err)
		_ = m.Store.AppendJobLog(ctx, jobID, "error", err.Error())
		_ = m.Store.FinishJob(ctx, jobID, "failed", err.Error())
		_ = m.Store.Notify(ctx, notifyID, "error", "Backup failed for "+site.Domain,
			"The backup could not be finished. Details are in the job log.")
		return
	}
	_ = m.Store.AppendJobLog(ctx, jobID, "info", "backup saved")
	_ = m.Store.FinishJob(ctx, jobID, "succeeded", "")
	if actorID != "" {
		_ = m.Store.Notify(ctx, notifyID, "success", "Backup ready", site.Domain+" was backed up.")
	}
}

func (m *Manager) backupSite(ctx context.Context, site store.Site, jobID string) error {
	output := BackupRoot + "/" + site.LinuxUser + "/" + time.Now().UTC().Format("20060102-150405") + ".tar.gz"
	_ = m.Store.AppendJobLog(ctx, jobID, "info", "creating backup of files and database")
	out, err := m.Helper.Output(ctx, "backup.create", map[string]string{
		"user": site.LinuxUser, "database": site.DBName, "docroot": DocRoot(site.LinuxUser), "output": output,
	})
	if err != nil {
		return err
	}
	path, size, err := parseBackupOutput(out)
	if err != nil {
		return err
	}
	return m.Store.InsertBackup(ctx, site.ID, size, path)
}

// parseBackupOutput reads the helper's "<path> <size_bytes>" reply. The path is
// later passed to backup.delete, so anything outside BackupRoot is refused.
func parseBackupOutput(out string) (string, int64, error) {
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return "", 0, errors.New("the backup helper sent an unexpected reply")
	}
	path := fields[0]
	if !strings.HasPrefix(path, BackupRoot+"/") || strings.Contains(path, "..") {
		return "", 0, errors.New("the backup helper reported a path outside the backup folder")
	}
	size, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || size < 0 {
		return "", 0, errors.New("the backup helper reported an invalid size")
	}
	return path, size, nil
}

// RunScheduledBackups backs up every live website once and then removes archives
// older than the retention period. It is called by the daily scheduler.
func (m *Manager) RunScheduledBackups(ctx context.Context) {
	settings, err := m.Store.GetBackupSettings(ctx)
	if err != nil {
		m.Log.Warn("scheduled backups skipped: settings unavailable", "error", err)
		return
	}
	sites, err := m.Store.ListReadySites(ctx)
	if err != nil {
		m.Log.Warn("scheduled backups skipped: listing sites failed", "error", err)
		return
	}
	for _, site := range sites {
		if ctx.Err() != nil {
			return
		}
		jobID, err := m.Store.CreateManagementJob(ctx, "", site.ID, site.NodeID, JobBackup)
		if err != nil {
			m.Log.Warn("scheduled backup job failed to start", "site_id", site.ID, "error", err)
			continue
		}
		bctx, cancel := context.WithTimeout(ctx, m.BackupTimeout)
		m.runBackup(bctx, site, jobID, "")
		cancel()
	}
	m.PruneBackups(ctx, time.Now(), settings.RetentionDays)
}

// PruneBackups deletes archives created before the retention cutoff. An archive
// whose deletion fails keeps its record, so the next run tries again.
func (m *Manager) PruneBackups(ctx context.Context, now time.Time, retentionDays int) {
	expired, err := m.Store.BackupsBefore(ctx, now.AddDate(0, 0, -retentionDays))
	if err != nil {
		m.Log.Warn("listing expired backups failed", "error", err)
		return
	}
	for _, b := range expired {
		if _, err := m.Helper.Do(ctx, "backup.delete", map[string]string{"path": b.Path}); err != nil {
			m.Log.Warn("deleting expired backup failed", "backup_id", b.ID, "error", err)
			continue
		}
		if err := m.Store.DeleteBackup(ctx, b.ID); err != nil {
			m.Log.Warn("removing expired backup record failed", "backup_id", b.ID, "error", err)
		}
	}
}

// RunAutoUpdates applies WordPress updates to every site that has auto-updates on.
// A failed site is reported to its owner and does not stop the others.
func (m *Manager) RunAutoUpdates(ctx context.Context) {
	sites, err := m.Store.ListAutoUpdateSites(ctx)
	if err != nil {
		m.Log.Warn("WordPress auto-updates skipped: listing sites failed", "error", err)
		return
	}
	for _, site := range sites {
		if ctx.Err() != nil {
			return
		}
		uctx, cancel := context.WithTimeout(ctx, m.UpdateTimeout)
		_, err := m.Helper.Do(uctx, "wp.update", map[string]string{
			"user": site.LinuxUser, "path": DocRoot(site.LinuxUser),
		})
		cancel()
		if err != nil {
			m.Log.Warn("WordPress auto-update failed", "site_id", site.ID, "error", err)
			_ = m.Store.Notify(ctx, site.OwnerID, "warning", "WordPress update failed for "+site.Domain,
				"The automatic update did not finish. Check the website and update it by hand if needed.")
		}
	}
}
