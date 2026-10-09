package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// progressSites serves one latest backup job and its steps on top of the settings fakes.
type progressSites struct {
	*settingsSites
	job      *store.Job
	steps    []store.JobStep
	jobType  string
	stepsFor string
}

func (p *progressSites) LatestJobForSiteOfType(_ context.Context, _, jobType string) (store.Job, error) {
	p.jobType = jobType
	if p.job == nil {
		return store.Job{}, store.ErrNotFound
	}
	return *p.job, nil
}

func (p *progressSites) JobSteps(_ context.Context, jobID string) ([]store.JobStep, error) {
	p.stepsFor = jobID
	return p.steps, nil
}

func newProgressEnv(t *testing.T, ps *progressSites) settingsEnv {
	t.Helper()
	e := newSettingsEnv(t)
	ps.settingsSites = e.sites
	e.h = NewRouter(Deps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:  e.fs,
		Sites:  ps,
		Manage: e.manage,
		Alerts: e.alert,
		Site:   SiteSettings{NodeID: "node-1", PHPVersion: "8.3", SecretKey: []byte(testServerSecret), StartJob: func(string) {}},
	})
	return e
}

type progressBody struct {
	JobID   string `json:"job_id"`
	Status  string `json:"status"`
	Percent int    `json:"percent"`
	Steps   []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"steps"`
}

func getProgress(t *testing.T, e settingsEnv, cookie *http.Cookie) (int, map[string]json.RawMessage, progressBody) {
	t.Helper()
	rec := e.call(t, http.MethodGet, "/api/v1/sites/33333333-3333-3333-3333-333333333333/backup-progress", "", cookie, false)
	var raw map[string]json.RawMessage
	var body progressBody
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, raw, body
}

func TestBackupProgressWithoutJobReturnsEmptyJobID(t *testing.T) {
	ps := &progressSites{}
	e := newProgressEnv(t, ps)
	admin := e.login(t, testEmail)

	code, raw, _ := getProgress(t, e, admin)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if len(raw) != 1 || string(raw["job_id"]) != `""` {
		t.Fatalf("body = %v, want only job_id empty", raw)
	}
	if ps.jobType != "site.backup" {
		t.Fatalf("job type queried = %q", ps.jobType)
	}
}

func TestBackupProgressRunningSFTPJob(t *testing.T) {
	ps := &progressSites{
		job: &store.Job{ID: "job-9", Type: "site.backup", Status: "running"},
		steps: []store.JobStep{
			{Name: "archive", Status: "succeeded"},
			{Name: "upload", Status: "running"},
			{Name: "record", Status: "pending"},
		},
	}
	e := newProgressEnv(t, ps)
	admin := e.login(t, testEmail)

	code, _, body := getProgress(t, e, admin)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if body.JobID != "job-9" || body.Status != "running" || body.Percent != 50 {
		t.Fatalf("body = %+v", body)
	}
	want := []string{"archive:succeeded", "upload:running", "record:pending"}
	for i, s := range body.Steps {
		if got := s.Name + ":" + s.Status; i >= len(want) || got != want[i] {
			t.Fatalf("step %d = %s, want %v", i, got, want)
		}
	}
	if len(body.Steps) != len(want) {
		t.Fatalf("steps = %+v", body.Steps)
	}
	if ps.stepsFor != "job-9" {
		t.Fatalf("steps read for %q", ps.stepsFor)
	}
}

func TestBackupProgressFailedLocalJob(t *testing.T) {
	ps := &progressSites{
		job: &store.Job{ID: "job-3", Type: "site.backup", Status: "failed"},
		steps: []store.JobStep{
			{Name: "archive", Status: "succeeded"},
			{Name: "record", Status: "failed", Error: "disk full"},
		},
	}
	e := newProgressEnv(t, ps)
	admin := e.login(t, testEmail)

	code, raw, body := getProgress(t, e, admin)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if body.Status != "failed" || body.Percent != 50 || len(body.Steps) != 2 {
		t.Fatalf("body = %+v", body)
	}
	if body.Steps[1].Name != "record" || body.Steps[1].Status != "failed" {
		t.Fatalf("steps = %+v", body.Steps)
	}
	if _, leaked := raw["steps"]; !leaked {
		t.Fatal("steps missing")
	}
	var stepRaw []map[string]any
	if err := json.Unmarshal(raw["steps"], &stepRaw); err != nil {
		t.Fatal(err)
	}
	for _, s := range stepRaw {
		if len(s) != 2 {
			t.Fatalf("step has extra fields: %v", s)
		}
	}
}

func TestBackupProgressCompletedLocalJobIsHundredPercent(t *testing.T) {
	ps := &progressSites{
		job: &store.Job{ID: "job-4", Type: "site.backup", Status: "succeeded"},
		steps: []store.JobStep{
			{Name: "archive", Status: "succeeded"},
			{Name: "record", Status: "succeeded"},
		},
	}
	e := newProgressEnv(t, ps)
	admin := e.login(t, testEmail)

	_, _, body := getProgress(t, e, admin)
	if body.Percent != 100 || body.Status != "succeeded" || len(body.Steps) != 2 {
		t.Fatalf("body = %+v", body)
	}
}

func TestBackupProgressRequiresSignIn(t *testing.T) {
	ps := &progressSites{}
	e := newProgressEnv(t, ps)
	if code, _, _ := getProgress(t, e, nil); code == http.StatusOK {
		t.Fatal("progress returned without a session")
	}
}
