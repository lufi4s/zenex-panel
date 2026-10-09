package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

const restorePath = "/api/v1/sites/33333333-3333-3333-3333-333333333333/backups/%d/restore"

// restoreSites serves one backup and reports which job types are running, on top of the progress fakes.
type restoreSites struct {
	*progressSites
	backup  store.Backup
	found   bool
	running map[string]bool
}

func (r *restoreSites) GetBackup(_ context.Context, _ string, id int64) (store.Backup, error) {
	if !r.found || id != r.backup.ID {
		return store.Backup{}, store.ErrNotFound
	}
	return r.backup, nil
}

func (r *restoreSites) LatestJobForSiteOfType(_ context.Context, _, jobType string) (store.Job, error) {
	if r.running[jobType] {
		return store.Job{ID: "job-live", Type: jobType, Status: "running"}, nil
	}
	return store.Job{}, store.ErrNotFound
}

// restoreManage records restore starts.
type restoreManage struct {
	settingsManage
	restored []int64
}

func (m *restoreManage) StartRestore(_ context.Context, _ store.Site, b store.Backup, _ string, _ string) (string, error) {
	m.restored = append(m.restored, b.ID)
	return "job-restore", nil
}

func newRestoreEnv(t *testing.T, rs *restoreSites, rm *restoreManage) settingsEnv {
	t.Helper()
	ps := &progressSites{}
	e := newProgressEnv(t, ps)
	e.sites.siteState = "ready"
	rs.progressSites = ps
	e.h = NewRouter(Deps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:  e.fs,
		Sites:  rs,
		Manage: rm,
		Alerts: e.alert,
		Site:   SiteSettings{NodeID: "node-1", PHPVersion: "8.3", SecretKey: []byte(testServerSecret), StartJob: func(string) {}},
	})
	return e
}

func TestRestoreUnknownBackupIsNotFound(t *testing.T) {
	rs := &restoreSites{}
	rm := &restoreManage{}
	e := newRestoreEnv(t, rs, rm)
	admin := e.login(t, testEmail)

	rec := e.call(t, http.MethodPost, fmt.Sprintf(restorePath, 7), "", admin, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if len(rm.restored) != 0 {
		t.Fatalf("restore started for an unknown backup: %v", rm.restored)
	}
}

func TestRestoreRefusedWhileBackupRuns(t *testing.T) {
	rs := &restoreSites{found: true, backup: store.Backup{ID: 7, Path: "/var/backups/zenex/zx_shop/a.tar.gz"}, running: map[string]bool{"site.backup": true}}
	rm := &restoreManage{}
	e := newRestoreEnv(t, rs, rm)
	admin := e.login(t, testEmail)

	rec := e.call(t, http.MethodPost, fmt.Sprintf(restorePath, 7), "", admin, true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if errorCode(t, rec) != "backup_in_progress" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if len(rm.restored) != 0 {
		t.Fatalf("restore started while a backup runs: %v", rm.restored)
	}
}

func TestRestoreStartsJobForOwnBackup(t *testing.T) {
	rs := &restoreSites{found: true, backup: store.Backup{ID: 7, Path: "/var/backups/zenex/zx_shop/a.tar.gz"}}
	rm := &restoreManage{}
	e := newRestoreEnv(t, rs, rm)
	admin := e.login(t, testEmail)

	rec := e.call(t, http.MethodPost, fmt.Sprintf(restorePath, 7), "", admin, true)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.JobID != "job-restore" || len(rm.restored) != 1 || rm.restored[0] != 7 {
		t.Fatalf("job = %q, restored = %v", body.JobID, rm.restored)
	}
}
