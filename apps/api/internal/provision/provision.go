// Package provision builds WordPress sites as a sequence of recorded, idempotent steps.
//
// Each step can be repeated safely. A failed step stops the job and records the
// exact error; a retry runs the steps again from the beginning, and each step
// skips work that is already done.
// Passwords are derived from the server secret and the site ID, so nothing
// secret is stored and a resumed job can always recompute them.
package provision

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// JobType is the job type stored in the jobs table.
const JobType = "site.provision"

// Steps in execution order. The names are stored in job_steps.
var stepNames = []string{
	"create_account",
	"prepare_files",
	"create_database",
	"php_pool",
	"download_wordpress",
	"create_wp_config",
	"install_wordpress",
	"harden_wordpress",
	"web_site",
	"health_check",
}

// StepNames returns the ordered step names (used to show progress).
func StepNames() []string { return append([]string(nil), stepNames...) }

// Store is the persistence surface the provisioner needs.
type Store interface {
	GetJob(ctx context.Context, id string) (store.Job, error)
	GetSite(ctx context.Context, id string) (store.Site, error)
	SiteOwnerEmail(ctx context.Context, siteID string) (string, error)
	SetSiteUID(ctx context.Context, siteID string, uid int) error
	SetSiteState(ctx context.Context, siteID, state string) error
	ClaimJob(ctx context.Context, jobID string) (bool, error)
	EnsureJobSteps(ctx context.Context, jobID string, names []string) error
	JobSteps(ctx context.Context, jobID string) ([]store.JobStep, error)
	SetStepStatus(ctx context.Context, jobID, name, status, errMsg string) error
	FinishJob(ctx context.Context, jobID, status, errMsg string) error
	AppendJobLog(ctx context.Context, jobID, level, msg string) error
}

// Helper runs privileged operations. Implemented by helperclient.Client.
type Helper interface {
	Do(ctx context.Context, op string, args map[string]string) (string, error)
}

// Provisioner runs provisioning jobs.
type Provisioner struct {
	Store     Store
	Helper    Helper
	SecretKey []byte
	// HealthBase is where the health check reaches the web server. Default http://127.0.0.1.
	HealthBase string
	Log        *slog.Logger
	HTTP       *http.Client
}

// New returns a provisioner with sensible defaults.
func New(s Store, h Helper, secret []byte, log *slog.Logger) *Provisioner {
	return &Provisioner{
		Store:      s,
		Helper:     h,
		SecretKey:  secret,
		HealthBase: "http://127.0.0.1",
		Log:        log,
		HTTP:       &http.Client{Timeout: 15 * time.Second},
	}
}

// DBPassword derives the site's database password.
func DBPassword(key []byte, siteID string) string {
	return derive(key, "db:"+siteID, 32)
}

// WPAdminPassword derives the WordPress administrator password for a site.
func WPAdminPassword(key []byte, siteID string) string {
	return derive(key, "wp-admin:"+siteID, 24)
}

func derive(key []byte, msg string, n int) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))[:n]
}

type run struct {
	job        store.Job
	site       store.Site
	ownerEmail string
	dbPass     string
	wpPass     string
}

type stepFunc func(ctx context.Context, p *Provisioner, r *run) error

var stepFuncs = map[string]stepFunc{
	"create_account":     stepCreateAccount,
	"prepare_files":      stepPrepareFiles,
	"create_database":    stepCreateDatabase,
	"php_pool":           stepPHPPool,
	"download_wordpress": stepDownloadWordPress,
	"create_wp_config":   stepCreateWPConfig,
	"install_wordpress":  stepInstallWordPress,
	"harden_wordpress":   stepHardenWordPress,
	"web_site":           stepWebSite,
	"health_check":       stepHealthCheck,
}

// Run executes a provisioning job from its first unfinished step. It returns nil
// when the job finished, or when another worker already owns it.
func (p *Provisioner) Run(ctx context.Context, jobID string) error {
	job, err := p.Store.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job: %w", err)
	}
	if job.Type != JobType || job.SiteID == "" {
		return fmt.Errorf("job %s is not a site provisioning job", jobID)
	}
	claimed, err := p.Store.ClaimJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("claim job: %w", err)
	}
	if !claimed {
		return nil
	}

	site, err := p.Store.GetSite(ctx, job.SiteID)
	if err != nil {
		return p.fail(ctx, jobID, "", nil, fmt.Errorf("load site: %w", err))
	}
	email, err := p.Store.SiteOwnerEmail(ctx, site.ID)
	if err != nil {
		return p.fail(ctx, jobID, "", nil, fmt.Errorf("load owner: %w", err))
	}
	if err := p.Store.EnsureJobSteps(ctx, jobID, stepNames); err != nil {
		return p.fail(ctx, jobID, "", nil, err)
	}
	if err := p.Store.SetSiteState(ctx, site.ID, "provisioning"); err != nil {
		return p.fail(ctx, jobID, "", nil, err)
	}

	r := &run{
		job:        job,
		site:       site,
		ownerEmail: email,
		dbPass:     DBPassword(p.SecretKey, site.ID),
		wpPass:     WPAdminPassword(p.SecretKey, site.ID),
	}
	secrets := []string{r.dbPass, r.wpPass}

	// Every step runs on every attempt. Each one checks the current state first
	// and does nothing if the work is already in place, so a retry also applies
	// any fix that changed a step's behaviour since the last attempt.
	for _, name := range stepNames {
		if err := p.Store.SetStepStatus(ctx, jobID, name, "running", ""); err != nil {
			return p.fail(ctx, jobID, name, secrets, err)
		}
		p.log(ctx, jobID, "info", "starting "+name)

		if err := stepFuncs[name](ctx, p, r); err != nil {
			return p.fail(ctx, jobID, name, secrets, err)
		}
		if err := p.Store.SetStepStatus(ctx, jobID, name, "succeeded", ""); err != nil {
			return p.fail(ctx, jobID, name, secrets, err)
		}
		p.log(ctx, jobID, "info", "finished "+name)
	}

	if err := p.Store.FinishJob(ctx, jobID, "succeeded", ""); err != nil {
		return err
	}
	return p.Store.SetSiteState(ctx, site.ID, "ready")
}

// ResumePending runs every provisioning job that was interrupted by a restart.
func (p *Provisioner) ResumePending(ctx context.Context, ids []string) {
	for _, id := range ids {
		if err := p.Run(ctx, id); err != nil {
			p.Log.Error("resume provisioning failed", "job_id", id, "error", err)
		}
	}
}

// fail records the failure on the step, the job and the site. The message is
// stripped of every derived secret before it is stored.
func (p *Provisioner) fail(ctx context.Context, jobID, step string, secrets []string, err error) error {
	msg := sanitize(err.Error(), secrets)
	if step != "" {
		_ = p.Store.SetStepStatus(ctx, jobID, step, "failed", msg)
	}
	_ = p.Store.FinishJob(ctx, jobID, "failed", msg)
	if job, jerr := p.Store.GetJob(ctx, jobID); jerr == nil && job.SiteID != "" {
		_ = p.Store.SetSiteState(ctx, job.SiteID, "failed")
	}
	p.log(ctx, jobID, "error", msg)
	p.Log.Warn("provisioning step failed", "job_id", jobID, "step", step, "error", msg)
	return fmt.Errorf("provisioning failed at %s: %s", step, msg)
}

func (p *Provisioner) log(ctx context.Context, jobID, level, msg string) {
	_ = p.Store.AppendJobLog(ctx, jobID, level, msg)
}

func sanitize(msg string, secrets []string) string {
	for _, s := range secrets {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "********")
		}
	}
	if len(msg) > 500 {
		msg = msg[:500] + "..."
	}
	return msg
}

// ---------------------------------------------------------------------------
// Steps
// ---------------------------------------------------------------------------

func stepCreateAccount(ctx context.Context, p *Provisioner, r *run) error {
	uidStr, err := p.Helper.Do(ctx, "user.create", map[string]string{"user": r.site.LinuxUser})
	if err != nil {
		return err
	}
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return errors.New("helper returned an invalid account id")
	}
	return p.Store.SetSiteUID(ctx, r.site.ID, uid)
}

func stepPrepareFiles(ctx context.Context, p *Provisioner, r *run) error {
	_, err := p.Helper.Do(ctx, "fs.prepare", map[string]string{"user": r.site.LinuxUser})
	return err
}

func stepCreateDatabase(ctx context.Context, p *Provisioner, r *run) error {
	_, err := p.Helper.Do(ctx, "db.create", map[string]string{
		"db": r.site.DBName, "dbuser": r.site.DBUser, "password": r.dbPass,
	})
	return err
}

func stepPHPPool(ctx context.Context, p *Provisioner, r *run) error {
	_, err := p.Helper.Do(ctx, "pool.write", map[string]string{
		"user": r.site.LinuxUser, "php": r.site.PHPVersion,
	})
	return err
}

func stepDownloadWordPress(ctx context.Context, p *Provisioner, r *run) error {
	_, err := p.Helper.Do(ctx, "wp.core-download", map[string]string{"user": r.site.LinuxUser})
	return err
}

func stepCreateWPConfig(ctx context.Context, p *Provisioner, r *run) error {
	_, err := p.Helper.Do(ctx, "wp.config-create", map[string]string{
		"user": r.site.LinuxUser, "db": r.site.DBName, "dbuser": r.site.DBUser, "dbpass": r.dbPass,
	})
	return err
}

func stepInstallWordPress(ctx context.Context, p *Provisioner, r *run) error {
	_, err := p.Helper.Do(ctx, "wp.core-install", map[string]string{
		"user":       r.site.LinuxUser,
		"url":        "http://" + r.site.Domain,
		"title":      "Zenex " + r.site.Slug,
		"admin":      "zenexadmin",
		"adminpass":  r.wpPass,
		"adminemail": r.ownerEmail,
	})
	return err
}

func stepHardenWordPress(ctx context.Context, p *Provisioner, r *run) error {
	_, err := p.Helper.Do(ctx, "wp.harden", map[string]string{"user": r.site.LinuxUser})
	return err
}

func stepWebSite(ctx context.Context, p *Provisioner, r *run) error {
	_, err := p.Helper.Do(ctx, "vhost.write", map[string]string{
		"user": r.site.LinuxUser, "domain": r.site.Domain,
	})
	return err
}

// stepHealthCheck asks the web server for the site using its domain name, so it works
// before DNS points at this server.
func stepHealthCheck(ctx context.Context, p *Provisioner, r *run) error {
	var last error
	for attempt := 1; attempt <= 5; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.HealthBase+"/", nil)
		if err != nil {
			return err
		}
		req.Host = r.site.Domain
		resp, err := p.HTTP.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return nil
			}
			last = fmt.Errorf("site answered with HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf("health check failed: %w", last)
}
