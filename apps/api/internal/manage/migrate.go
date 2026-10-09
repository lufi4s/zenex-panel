package manage

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// JobMigrate is the job type recorded for a migration from cPanel.
const JobMigrate = "site.migrate"

// Migration steps, in order. The first waits for the new website to be built here.
const (
	StepMigrateSite     = "create_site"
	StepMigrateDownload = "download"
	StepMigrateVerify   = "verify"
	// The restore step is StepRestore: the archive is put in place by the same code as a backup restore.
)

// migratePoll is how often a migration checks whether the new website is built.
var migratePoll = 3 * time.Second

// CpanelCreds are the SSH sign-in details of a cPanel account. They live in memory for one
// request or one migration and are never stored.
type CpanelCreds struct {
	Host     string
	Port     int
	Username string
	Password string
}

func (c CpanelCreds) args() map[string]string {
	return map[string]string{
		"host": c.Host, "port": strconv.Itoa(c.Port), "username": c.Username, "password": c.Password,
	}
}

// CpanelInstall is one WordPress install found on a cPanel account.
type CpanelInstall struct {
	Path        string `json:"path"`
	SiteURL     string `json:"site_url"`
	Domain      string `json:"domain"`
	DBName      string `json:"db_name"`
	TablePrefix string `json:"table_prefix"`
	SizeKB      int64  `json:"size_kb"`
}

// ScanCpanel lists the WordPress installs of a cPanel account. It only reads.
func (m *Manager) ScanCpanel(ctx context.Context, c CpanelCreds) ([]CpanelInstall, error) {
	out, err := m.Helper.Output(ctx, "cpanel.scan", c.args())
	if err != nil {
		return nil, redactError(err, c.Password)
	}
	installs := []CpanelInstall{}
	if err := json.Unmarshal([]byte(out), &installs); err != nil {
		return nil, errors.New("the cPanel scan returned a reply the panel could not read")
	}
	return installs, nil
}

// MigrationRequest describes one website to bring over from cPanel.
type MigrationRequest struct {
	// Site is the website on this panel that receives the data. It is built or ready.
	Site store.Site
	// ProvisionJobID is the build job of a newly created Site. Empty when Site already exists.
	ProvisionJobID string
	Creds          CpanelCreds
	// SourcePath is the website folder on the cPanel account.
	SourcePath string
	// SourceDomain is the address the website had on cPanel. When it differs from the new
	// website's domain, the stored addresses are changed.
	SourceDomain string
	ActorID      string
}

// MigrationSteps are the ordered step names of a migration job.
func MigrationSteps() []string {
	return []string{StepMigrateSite, StepMigrateDownload, StepRestore, StepMigrateVerify}
}

// StartMigration records the migration job and runs it in the background. It returns the job ID.
func (m *Manager) StartMigration(ctx context.Context, req MigrationRequest) (string, error) {
	if m.DBPassword == nil {
		return "", errors.New("the database password cannot be derived on this server")
	}
	jobID, err := m.Store.CreateManagementJob(ctx, req.ActorID, req.Site.ID, req.Site.NodeID, JobMigrate)
	if err != nil {
		return "", err
	}
	if err := m.Store.EnsureJobSteps(ctx, jobID, MigrationSteps()); err != nil {
		_ = m.Store.FinishJob(ctx, jobID, "failed", "the migration steps could not be recorded")
		return "", err
	}
	go func() {
		mctx, cancel := context.WithTimeout(context.Background(), m.MigrateTimeout)
		defer cancel()
		m.runMigration(mctx, req, jobID)
	}()
	return jobID, nil
}

// runMigration runs the migration and records how it ended. The actor is told either way.
func (m *Manager) runMigration(ctx context.Context, req MigrationRequest, jobID string) {
	site := req.Site
	if err := m.migrateSite(ctx, req, jobID); err != nil {
		msg := redactError(err, req.Creds.Password).Error()
		m.Log.Warn("cPanel migration failed", "site_id", site.ID, "error", msg)
		_ = m.Store.AppendJobLog(ctx, jobID, "error", msg)
		_ = m.Store.FinishJob(ctx, jobID, "failed", msg)
		_ = m.Store.Notify(ctx, req.ActorID, "error", "Migration failed for "+site.Domain,
			"The website was not moved over. The cPanel account was not changed. Details are in the job log; you can retry.")
		return
	}
	_ = m.Store.AppendJobLog(ctx, jobID, "info", "the website was moved over")
	_ = m.Store.FinishJob(ctx, jobID, "succeeded", "")
	next := "Point the DNS of " + site.Domain + " to this server to make it live."
	if m.PublicIP != "" {
		next = "Point the DNS A record of " + site.Domain + " to " + m.PublicIP + " to make it live."
	}
	_ = m.Store.Notify(ctx, req.ActorID, "success", "Migration finished", site.Domain+" was copied from cPanel. "+next)
}

// migrateSite runs the steps in order. The first failing step stops the migration.
func (m *Manager) migrateSite(ctx context.Context, req MigrationRequest, jobID string) error {
	site := req.Site

	if err := m.runStep(ctx, jobID, StepMigrateSite, func() error {
		return m.waitForSite(ctx, jobID, req.ProvisionJobID)
	}); err != nil {
		return err
	}

	var archive string
	var size int64
	if err := m.runStep(ctx, jobID, StepMigrateDownload, func() error {
		_ = m.Store.AppendJobLog(ctx, jobID, "info", "copying the files and the database from the cPanel account; nothing is changed there")
		args := req.Creds.args()
		args["path"] = req.SourcePath
		args["user"] = site.LinuxUser
		args["output"] = BackupRoot + "/" + site.LinuxUser + "/migrate-" + time.Now().UTC().Format("20060102-150405") + ".tar.gz"
		out, err := m.Helper.Output(ctx, "cpanel.pull", args)
		if err != nil {
			return redactError(err, req.Creds.Password)
		}
		archive, size, err = parseBackupOutput(out)
		return err
	}); err != nil {
		return err
	}

	if err := m.runStep(ctx, jobID, StepRestore, func() error {
		_ = m.Store.AppendJobLog(ctx, jobID, "info", "putting the files and the database in place")
		_, err := m.Helper.Do(ctx, "backup.restore", m.restoreArgs(site, archive, req.SourceDomain))
		return err
	}); err != nil {
		// The copy is large and useless on its own, so it does not stay on the disk.
		m.removeLocalArchive(ctx, jobID, archive)
		return err
	}
	// The copy stays as a backup, so the migration can be undone from the backup list.
	if err := m.Store.InsertBackup(ctx, site.ID, size, archive); err != nil {
		m.Log.Warn("recording the migration copy failed", "site_id", site.ID, "error", err)
		_ = m.Store.AppendJobLog(ctx, jobID, "warning", "the copy of the website could not be listed as a backup")
	}

	return m.runStep(ctx, jobID, StepMigrateVerify, func() error {
		home, err := m.Helper.Output(ctx, "wp.verify", map[string]string{"user": site.LinuxUser})
		if err != nil {
			return err
		}
		_ = m.Store.AppendJobLog(ctx, jobID, "info", "WordPress runs and reads its database; its address is "+strings.TrimSpace(home))
		return nil
	})
}

// waitForSite waits for the build job of a new website to finish. With no job, the website
// already exists and there is nothing to wait for.
func (m *Manager) waitForSite(ctx context.Context, jobID, provisionJobID string) error {
	if provisionJobID == "" {
		return nil
	}
	_ = m.Store.AppendJobLog(ctx, jobID, "info", "waiting for the new website to be built")
	for {
		job, err := m.Store.GetJob(ctx, provisionJobID)
		if err != nil {
			return err
		}
		switch job.Status {
		case "succeeded":
			return nil
		case "failed", "dead", "cancelled":
			msg := "the new website could not be built"
			if job.Error != "" {
				msg += ": " + job.Error
			}
			return errors.New(msg)
		}
		select {
		case <-ctx.Done():
			return errors.New("the new website was not built in time")
		case <-time.After(migratePoll):
		}
	}
}
