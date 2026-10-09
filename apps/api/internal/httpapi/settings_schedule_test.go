package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

const runNowPath = "/api/v1/settings/backups/run-now"

func TestBackupFrequencyAndWeekdayValidation(t *testing.T) {
	ok := []store.BackupSettings{
		{Frequency: "hourly", ScheduleHour: 23, Weekday: 6, RetentionDays: 7},
		{Frequency: "daily", ScheduleHour: 0, Weekday: 0, RetentionDays: 7},
		{Frequency: "weekly", ScheduleHour: 3, Weekday: 0, RetentionDays: 7},
		{Frequency: " Weekly ", ScheduleHour: 3, Weekday: 6, RetentionDays: 7},
		{Frequency: "", ScheduleHour: 3, Weekday: 2, RetentionDays: 7},
	}
	for _, in := range ok {
		got, problem := validBackupSettings(in)
		if problem != "" {
			t.Errorf("%+v rejected: %s", in, problem)
		}
		if got.Frequency == "" || got.Frequency != store.BackupFrequencyDaily && got.Frequency != store.BackupFrequencyWeekly && got.Frequency != store.BackupFrequencyHourly {
			t.Errorf("%+v normalised to frequency %q", in, got.Frequency)
		}
	}
	if got, _ := validBackupSettings(store.BackupSettings{ScheduleHour: 3, RetentionDays: 7}); got.Frequency != store.BackupFrequencyDaily {
		t.Errorf("missing frequency normalised to %q, want daily", got.Frequency)
	}
	bad := map[string]store.BackupSettings{
		"unknown frequency": {Frequency: "monthly", ScheduleHour: 3, RetentionDays: 7},
		"weekday 7":         {Frequency: "weekly", ScheduleHour: 3, Weekday: 7, RetentionDays: 7},
		"weekday -1":        {Frequency: "weekly", ScheduleHour: 3, Weekday: -1, RetentionDays: 7},
	}
	for name, in := range bad {
		if _, problem := validBackupSettings(in); problem == "" {
			t.Errorf("%s: accepted %+v", name, in)
		}
	}
}

func TestBackupFrequencyPUTValidationAndGETShape(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)

	rec := e.call(t, http.MethodPut, "/api/v1/settings/backups", `{"frequency":"monthly","schedule_hour":3,"retention_days":7}`, admin, true)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_backup_settings" {
		t.Fatalf("unknown frequency = %d %s", rec.Code, rec.Body.String())
	}
	rec = e.call(t, http.MethodPut, "/api/v1/settings/backups", `{"frequency":"weekly","schedule_hour":3,"weekday":7,"retention_days":7}`, admin, true)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_backup_settings" {
		t.Fatalf("weekday 7 = %d %s", rec.Code, rec.Body.String())
	}
	rec = e.call(t, http.MethodPut, "/api/v1/settings/backups", `{"frequency":"weekly","schedule_hour":4,"weekday":5,"retention_days":14}`, admin, true)
	if rec.Code != http.StatusOK || e.sites.backup.Frequency != "weekly" || e.sites.backup.Weekday != 5 || e.sites.backup.ScheduleHour != 4 {
		t.Fatalf("weekly save = %d %+v", rec.Code, e.sites.backup)
	}

	rec = e.call(t, http.MethodGet, "/api/v1/settings/backups", "", admin, false)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["frequency"] != "weekly" || got["weekday"] != float64(5) || got["schedule_hour"] != float64(4) {
		t.Fatalf("GET = %s", rec.Body.String())
	}

	// A PUT without frequency (older clients) saves daily.
	rec = e.call(t, http.MethodPut, "/api/v1/settings/backups", `{"schedule_hour":2,"retention_days":7}`, admin, true)
	if rec.Code != http.StatusOK || e.sites.backup.Frequency != "daily" {
		t.Fatalf("PUT without frequency = %d %+v", rec.Code, e.sites.backup)
	}
}

func TestRunBackupsNowStartsReadySitesAndSkipsBusyOnes(t *testing.T) {
	e := newSettingsEnv(t)
	e.sites.sites = []store.Site{
		{ID: "s-ready-1", State: "ready"},
		{ID: "s-busy", State: "ready"},
		{ID: "s-ready-2", State: "ready"},
		{ID: "s-suspended", State: "suspended"},
		{ID: "s-fails", State: "ready"},
	}
	e.sites.active = []string{"s-busy"}
	e.manage.startErr = map[string]error{"s-fails": errors.New("helper down")}
	admin := e.login(t, testEmail)

	rec := e.call(t, http.MethodPost, runNowPath, "", admin, true)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("run-now = %d %s", rec.Code, rec.Body.String())
	}
	var res map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["started"] != 2 || res["skipped"] != 2 {
		t.Fatalf("counts = %v, want started 2 skipped 2", res)
	}
	if len(e.manage.started) != 2 || e.manage.started[0] != "s-ready-1" || e.manage.started[1] != "s-ready-2" {
		t.Fatalf("started sites = %v", e.manage.started)
	}
	if n := len(e.fs.audits); n == 0 || e.fs.audits[n-1].Action != "backup.run_now" {
		t.Fatalf("audit = %+v, want backup.run_now", e.fs.audits)
	}
}

func TestRunBackupsNowWithNoReadySitesReturnsZero(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	rec := e.call(t, http.MethodPost, runNowPath, "", admin, true)
	var res map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || rec.Code != http.StatusAccepted || res["started"] != 0 || res["skipped"] != 0 {
		t.Fatalf("empty run-now = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRunBackupsNowIsAdminOnlyAndNeedsCSRF(t *testing.T) {
	e := newSettingsEnv(t)
	e.sites.sites = []store.Site{{ID: "s-1", State: "ready"}}
	customer := e.login(t, "customer@example.com")
	if rec := e.call(t, http.MethodPost, runNowPath, "", customer, true); rec.Code != http.StatusForbidden {
		t.Fatalf("customer run-now = %d", rec.Code)
	}
	admin := e.login(t, testEmail)
	if rec := e.call(t, http.MethodPost, runNowPath, "", admin, false); rec.Code == http.StatusAccepted {
		t.Fatalf("run-now without CSRF accepted: %d", rec.Code)
	}
	if rec := e.call(t, http.MethodPost, runNowPath, "", nil, true); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous run-now = %d", rec.Code)
	}
	if len(e.manage.started) != 0 {
		t.Fatalf("rejected requests started backups: %v", e.manage.started)
	}
}
