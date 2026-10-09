package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// activitySites serves a fixed activity list and records who asked.
type activitySites struct {
	*progressSites
	list  []store.SiteActivity
	owner *string
	types []string
}

func (a *activitySites) ListSiteActivity(_ context.Context, owner string, types []string, _ time.Duration) ([]store.SiteActivity, error) {
	a.owner = &owner
	a.types = types
	return a.list, nil
}

func newActivityEnv(t *testing.T, list []store.SiteActivity) (settingsEnv, *activitySites) {
	t.Helper()
	ps := &progressSites{}
	e := newProgressEnv(t, ps)
	as := &activitySites{progressSites: ps, list: list}
	e.h = NewRouter(Deps{
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Users:  e.fs,
		Sites:  as,
		Manage: e.manage,
		Alerts: e.alert,
		Site:   SiteSettings{NodeID: "node-1", PHPVersion: "8.3", SecretKey: []byte(testServerSecret), StartJob: func(string) {}},
	})
	return e, as
}

type activityBody struct {
	SiteID  string `json:"site_id"`
	Type    string `json:"type"`
	Status  string `json:"status"`
	Percent int    `json:"percent"`
	Step    string `json:"step"`
}

func TestSiteActivityShowsProgressAndStep(t *testing.T) {
	e, as := newActivityEnv(t, []store.SiteActivity{
		{SiteID: "s1", JobID: "j1", Type: "site.backup", Status: "running", Steps: []store.ActivityStep{
			{Name: "archive", Status: "succeeded"}, {Name: "upload", Status: "running"}, {Name: "record", Status: "pending"},
		}},
		{SiteID: "s2", JobID: "j2", Type: "site.backup", Status: "succeeded", Steps: []store.ActivityStep{
			{Name: "archive", Status: "succeeded"}, {Name: "record", Status: "succeeded"},
		}},
		{SiteID: "s3", JobID: "j3", Type: "site.restore", Status: "running", Steps: []store.ActivityStep{
			{Name: "save_current", Status: "succeeded"}, {Name: "restore", Status: "pending"},
		}},
		{SiteID: "s4", JobID: "j4", Type: "site.migrate", Status: "failed", Steps: []store.ActivityStep{
			{Name: "create_site", Status: "succeeded"}, {Name: "download", Status: "failed"},
		}},
	})
	admin := e.login(t, testEmail)

	rec := e.call(t, http.MethodGet, "/api/v1/sites/activity", "", admin, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got []activityBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("entries = %d", len(got))
	}
	if got[0].Percent != 50 || got[0].Step != "upload" || got[0].Status != "running" {
		t.Fatalf("running backup = %+v", got[0])
	}
	if got[1].Percent != 100 || got[1].Step != "" {
		t.Fatalf("finished backup = %+v", got[1])
	}
	if got[2].Percent != 50 || got[2].Step != "restore" {
		t.Fatalf("restore = %+v (the next pending step is shown when none runs)", got[2])
	}
	if got[3].Status != "failed" || got[3].Step != "" {
		t.Fatalf("failed migration = %+v", got[3])
	}
	if as.owner == nil || *as.owner != "" {
		t.Fatalf("an administrator should see every website, owner filter = %v", as.owner)
	}
	if len(as.types) < 5 {
		t.Fatalf("job types = %v", as.types)
	}
}

func TestSiteActivityIsLimitedToTheCallersWebsites(t *testing.T) {
	e, as := newActivityEnv(t, nil)
	customer := e.login(t, "customer@example.com")

	rec := e.call(t, http.MethodGet, "/api/v1/sites/activity", "", customer, false)
	if rec.Code != http.StatusOK || rec.Body.String() == "null\n" {
		t.Fatalf("status = %d body = %q (an empty list must be [], not null)", rec.Code, rec.Body.String())
	}
	if as.owner == nil || *as.owner != "55555555-5555-5555-5555-555555555555" {
		t.Fatalf("owner filter = %v", as.owner)
	}
}

func TestSiteActivityNeedsSignIn(t *testing.T) {
	e, _ := newActivityEnv(t, nil)
	if rec := e.call(t, http.MethodGet, "/api/v1/sites/activity", "", nil, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}
