package manage

import (
	"context"
	"encoding/base64"
	"errors"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/helperclient"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// JobBackup is the job type recorded for a website backup.
const JobBackup = "site.backup"

// Backup job steps, in execution order. The upload step exists only for SFTP destinations.
const (
	StepArchive = "archive"
	StepUpload  = "upload"
	StepRecord  = "record"
)

const (
	// WebRoot is where the helper keeps each website's account home.
	WebRoot = "/var/www"
	// BackupRoot holds the archives. The helper only writes and deletes below it.
	BackupRoot = "/var/backups/zenex"

	defaultBackupTimeout  = 20 * time.Minute
	defaultRestoreTimeout = 40 * time.Minute
	defaultMigrateTimeout = 4 * time.Hour
	defaultUpdateTimeout  = 15 * time.Minute
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
	jobID, err := m.createBackupJob(ctx, actorID, site)
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

// BackupSteps returns the ordered step names of a backup job for a destination.
func BackupSteps(dest store.BackupDestination) []string {
	if dest.Type == store.BackupDestSFTP {
		return []string{StepArchive, StepUpload, StepRecord}
	}
	return []string{StepArchive, StepRecord}
}

// createBackupJob records the backup job and all its steps as pending, so the
// progress is visible before any work starts.
func (m *Manager) createBackupJob(ctx context.Context, actorID string, site store.Site) (string, error) {
	settings, err := m.Store.GetBackupSettings(ctx)
	if err != nil {
		return "", err
	}
	jobID, err := m.Store.CreateManagementJob(ctx, actorID, site.ID, site.NodeID, JobBackup)
	if err != nil {
		return "", err
	}
	if err := m.Store.EnsureJobSteps(ctx, jobID, BackupSteps(settings.Destination)); err != nil {
		_ = m.Store.FinishJob(ctx, jobID, "failed", "the backup steps could not be recorded")
		return "", err
	}
	return jobID, nil
}

// ProgressPercent is how far a job has got, from its step statuses: a succeeded step
// counts as one step and a running step as half of one. It returns 0 to 100.
func ProgressPercent(statuses []string) int {
	if len(statuses) == 0 {
		return 0
	}
	halves := 0
	for _, s := range statuses {
		switch s {
		case "succeeded":
			halves += 2
		case "running":
			halves++
		}
	}
	return halves * 100 / (2 * len(statuses))
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

// backupSite runs the archive, upload (SFTP only) and record steps in order. The first
// failing step stops the backup and its error is returned.
func (m *Manager) backupSite(ctx context.Context, site store.Site, jobID string) error {
	settings, err := m.Store.GetBackupSettings(ctx)
	if err != nil {
		return m.runStep(ctx, jobID, StepArchive, func() error { return err })
	}

	var archive string
	var size int64
	if err := m.runStep(ctx, jobID, StepArchive, func() error {
		var err error
		archive, size, err = m.makeArchive(ctx, jobID, site)
		return err
	}); err != nil {
		return err
	}

	location := archive
	if settings.Destination.Type == store.BackupDestSFTP {
		if err := m.runStep(ctx, jobID, StepUpload, func() error {
			remote, err := m.sendToSFTP(ctx, jobID, archive, settings.Destination.SFTP)
			if err != nil {
				return err
			}
			location = remote
			return nil
		}); err != nil {
			return err
		}
	}

	return m.runStep(ctx, jobID, StepRecord, func() error {
		if err := m.Store.InsertBackup(ctx, site.ID, size, location); err != nil {
			return err
		}
		return m.PruneBackups(ctx, time.Now(), settings.RetentionDays)
	})
}

// makeArchive writes a local archive of the website files and database. It returns the
// archive path and size. The archive stays on this server; the caller decides what to do with it.
func (m *Manager) makeArchive(ctx context.Context, jobID string, site store.Site) (string, int64, error) {
	output := BackupRoot + "/" + site.LinuxUser + "/" + time.Now().UTC().Format("20060102-150405") + ".tar.gz"
	_ = m.Store.AppendJobLog(ctx, jobID, "info", "creating backup of files and database")
	out, err := m.Helper.Output(ctx, "backup.create", map[string]string{
		"user": site.LinuxUser, "database": site.DBName, "docroot": DocRoot(site.LinuxUser), "output": output,
		"domain": site.Domain, "site_id": site.ID,
	})
	if err != nil {
		return "", 0, err
	}
	return parseBackupOutput(out)
}

// runStep marks a step running, runs its work and then marks it succeeded or failed.
// A failure is kept on the step; the caller decides what happens to the job.
func (m *Manager) runStep(ctx context.Context, jobID, name string, work func() error) error {
	if err := m.Store.SetStepStatus(ctx, jobID, name, "running", ""); err != nil {
		return err
	}
	if err := work(); err != nil {
		_ = m.Store.SetStepStatus(ctx, jobID, name, "failed", err.Error())
		return err
	}
	return m.Store.SetStepStatus(ctx, jobID, name, "succeeded", "")
}

// sendToSFTP uploads the local archive and then removes the local copy. It returns
// the remote location, which is what the backup record stores. When the upload
// fails, the local archive is removed too, because no record would point to it.
func (m *Manager) sendToSFTP(ctx context.Context, jobID, local string, dest store.SFTPDestination) (string, error) {
	remoteDir := path.Clean(dest.Path)
	_ = m.Store.AppendJobLog(ctx, jobID, "info", "sending the archive to the SFTP server")
	args := sftpArgs(dest, remoteDir)
	args["file"] = local
	err := m.runSFTPOp(ctx, "backup.upload", dest, args)
	m.removeLocalArchive(ctx, jobID, local)
	if err != nil {
		return "", err
	}
	return remoteBackupPath(dest.Host, path.Join(remoteDir, path.Base(local))), nil
}

// runSFTPOp runs a helper operation that talks to the SFTP server. The saved password
// is added to the arguments when the destination signs in with one. Any error text is
// returned without the password.
func (m *Manager) runSFTPOp(ctx context.Context, op string, dest store.SFTPDestination, args map[string]string) error {
	password, err := m.addSFTPPassword(ctx, dest, args)
	if err != nil {
		return err
	}
	_, err = m.Helper.Do(ctx, op, args)
	return redactError(err, password)
}

// runSFTPOutput is runSFTPOp for operations that return text, such as the backup listing.
// Errors are redacted the same way.
func (m *Manager) runSFTPOutput(ctx context.Context, op string, dest store.SFTPDestination, args map[string]string) (string, error) {
	password, err := m.addSFTPPassword(ctx, dest, args)
	if err != nil {
		return "", err
	}
	out, err := m.Helper.Output(ctx, op, args)
	return out, redactError(err, password)
}

// addSFTPPassword adds the "password" argument when dest uses password sign-in. It
// returns the password so the caller can redact it from errors. Key sign-in adds nothing.
func (m *Manager) addSFTPPassword(ctx context.Context, dest store.SFTPDestination, args map[string]string) (string, error) {
	if dest.Auth != store.SFTPAuthPassword {
		return "", nil
	}
	if m.SFTPPassword == nil {
		return "", errors.New("the SFTP password cannot be read on this server")
	}
	password, err := m.SFTPPassword(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return "", errors.New("no SFTP password is saved; enter it in the backup settings")
	}
	if err != nil {
		return "", err
	}
	if password == "" {
		return "", errors.New("no SFTP password is saved; enter it in the backup settings")
	}
	args["password"] = password
	return password, nil
}

// redactError removes the secret from an error message. Helper errors keep their type,
// because callers show their message to the customer.
func redactError(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	const mask = "[redacted]"
	var helperErr *helperclient.Error
	if errors.As(err, &helperErr) {
		return &helperclient.Error{Message: strings.ReplaceAll(helperErr.Message, secret, mask)}
	}
	if msg := err.Error(); strings.Contains(msg, secret) {
		return errors.New(strings.ReplaceAll(msg, secret, mask))
	}
	return err
}

// removeLocalArchive deletes a local archive. A failure is logged to the job only:
// the backup itself is already stored elsewhere or has failed.
func (m *Manager) removeLocalArchive(ctx context.Context, jobID, local string) {
	if _, err := m.Helper.Do(ctx, "backup.delete", map[string]string{"path": local}); err != nil {
		m.Log.Warn("removing local archive after SFTP step failed", "error", err)
		_ = m.Store.AppendJobLog(ctx, jobID, "warning", "the local copy of the archive could not be removed")
	}
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
		jobID, err := m.createBackupJob(ctx, "", site)
		if err != nil {
			m.Log.Warn("scheduled backup job failed to start", "site_id", site.ID, "error", err)
			continue
		}
		bctx, cancel := context.WithTimeout(ctx, m.BackupTimeout)
		m.runBackup(bctx, site, jobID, "")
		cancel()
	}
	if err := m.PruneBackups(ctx, time.Now(), settings.RetentionDays); err != nil {
		m.Log.Warn("pruning expired backups failed", "error", err)
	}
}

// PruneBackups deletes archives created before the retention cutoff. An archive
// whose deletion fails keeps its record, so the next run tries again. It returns an
// error only when the expired list cannot be read.
func (m *Manager) PruneBackups(ctx context.Context, now time.Time, retentionDays int) error {
	expired, err := m.Store.BackupsBefore(ctx, now.AddDate(0, 0, -retentionDays))
	if err != nil {
		m.Log.Warn("listing expired backups failed", "error", err)
		return err
	}
	settings, settingsErr := m.Store.GetBackupSettings(ctx)
	for _, b := range expired {
		args, err := deleteArgs(b.Path, settings, settingsErr)
		if err != nil {
			m.Log.Warn("expired backup skipped", "backup_id", b.ID, "error", err)
			continue
		}
		if err := m.runSFTPOpIfRemote(ctx, args, settings.Destination.SFTP, "backup.delete"); err != nil {
			m.Log.Warn("deleting expired backup failed", "backup_id", b.ID, "error", err)
			continue
		}
		if err := m.Store.DeleteBackup(ctx, b.ID); err != nil {
			m.Log.Warn("removing expired backup record failed", "backup_id", b.ID, "error", err)
		}
	}
	return nil
}

// runSFTPOpIfRemote runs a delete for a remote archive. Local deletes are plain helper calls
// without any SFTP password.
func (m *Manager) runSFTPOpIfRemote(ctx context.Context, args map[string]string, dest store.SFTPDestination, op string) error {
	if args["remote"] != "1" {
		_, err := m.Helper.Do(ctx, op, args)
		return err
	}
	return m.runSFTPOp(ctx, op, dest, args)
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

// remotePrefix marks a backup record whose archive is on an SFTP server.
// The form is "sftp://<host><absolute remote path>".
const remotePrefix = "sftp://"

// remoteBackupPath is the record path for an archive stored on an SFTP server.
func remoteBackupPath(host, remoteFile string) string {
	return remotePrefix + host + remoteFile
}

// parseRemoteBackupPath splits a remote record path. It reports false for local paths.
func parseRemoteBackupPath(p string) (host, remoteFile string, remote bool, err error) {
	rest, ok := strings.CutPrefix(p, remotePrefix)
	if !ok {
		return "", "", false, nil
	}
	host, file, ok := strings.Cut(rest, "/")
	if !ok || host == "" || file == "" || strings.Contains(file, "..") {
		return "", "", true, errors.New("a remote backup record has an invalid location")
	}
	return host, "/" + file, true, nil
}

// deleteArgs returns the backup.delete arguments for an archive. Remote archives
// use the SFTP host from the record and the port and username from the saved settings.
func deleteArgs(backupPath string, settings store.BackupSettings, settingsErr error) (map[string]string, error) {
	host, remoteFile, remote, err := parseRemoteBackupPath(backupPath)
	if err != nil {
		return nil, err
	}
	if !remote {
		return map[string]string{"path": backupPath}, nil
	}
	if settingsErr != nil {
		return nil, settingsErr
	}
	dest := settings.Destination.SFTP
	dest.Host = host
	args := sftpArgs(dest, remoteFile)
	args["remote"] = "1"
	return args, nil
}

// sftpArgs are the connection fields the helper needs. path is the remote directory
// for uploads and tests, or the remote file for deletes.
func sftpArgs(dest store.SFTPDestination, remotePath string) map[string]string {
	return map[string]string{
		"host":     dest.Host,
		"port":     strconv.Itoa(dest.Port),
		"username": dest.Username,
		"path":     remotePath,
	}
}

// SFTPPublicKey creates the backup key pair on this server if it does not exist yet
// and returns the public key line to install in the remote account's authorized_keys.
func (m *Manager) SFTPPublicKey(ctx context.Context) (string, error) {
	out, err := m.Helper.Output(ctx, "backup.keygen", map[string]string{})
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(out)
	if !validPublicKey(key) {
		return "", errors.New("the backup helper sent an unexpected public key")
	}
	return key, nil
}

// validPublicKey accepts one ed25519 public key line with a comment.
func validPublicKey(key string) bool {
	fields := strings.Fields(key)
	if len(fields) != 3 || len(key) > 1024 || fields[0] != "ssh-ed25519" || !strings.HasPrefix(fields[1], "AAAA") {
		return false
	}
	_, err := base64.StdEncoding.DecodeString(fields[1])
	return err == nil
}

// TestSFTP checks that the panel can sign in to the SFTP server and reach the path.
func (m *Manager) TestSFTP(ctx context.Context, dest store.SFTPDestination) error {
	return m.runSFTPOp(ctx, "backup.test", dest, sftpArgs(dest, dest.Path))
}
