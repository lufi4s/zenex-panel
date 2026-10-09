package manage

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

func stepNamesAndStatus(steps []store.JobStep) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Name+"="+s.Status)
	}
	return out
}

func TestBackupStepsDependOnDestination(t *testing.T) {
	if got := BackupSteps(store.BackupDestination{Type: store.BackupDestLocal}); !reflect.DeepEqual(got, []string{"archive", "record"}) {
		t.Fatalf("local steps = %v", got)
	}
	if got := BackupSteps(store.BackupDestination{Type: store.BackupDestSFTP}); !reflect.DeepEqual(got, []string{"archive", "upload", "record"}) {
		t.Fatalf("sftp steps = %v", got)
	}
}

func TestCreateBackupJobRecordsAllStepsPending(t *testing.T) {
	m, st, _ := newFeatureTest("ready")
	st.settings = sftpSettings()
	site := store.Site{ID: "s1", NodeID: "n1", State: "ready"}
	jobID, err := m.createBackupJob(context.Background(), "u1", site)
	if err != nil || jobID != "job-1" {
		t.Fatalf("createBackupJob = %q %v", jobID, err)
	}
	want := []string{"archive=pending", "upload=pending", "record=pending"}
	if got := stepNamesAndStatus(st.steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	if len(st.stepEvent) != 0 {
		t.Fatalf("steps started before the work: %v", st.stepEvent)
	}
}

func TestCreateBackupJobLocalHasNoUploadStep(t *testing.T) {
	m, st, _ := newFeatureTest("ready")
	if _, err := m.createBackupJob(context.Background(), "", store.Site{ID: "s1"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"archive=pending", "record=pending"}
	if got := stepNamesAndStatus(st.steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
}

func TestLocalBackupStepTransitions(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	h.outputs["backup.create"] = BackupRoot + "/zx_shop/x.tar.gz 2048"
	site := store.Site{ID: "s1", NodeID: "n1", LinuxUser: "zx_shop", OwnerID: "u1", State: "ready"}
	jobID, err := m.createBackupJob(context.Background(), "u1", site)
	if err != nil {
		t.Fatal(err)
	}

	m.runBackup(context.Background(), site, jobID, "u1")

	want := []string{"archive:running", "archive:succeeded", "record:running", "record:succeeded"}
	if !reflect.DeepEqual(st.stepEvent, want) {
		t.Fatalf("step events = %v, want %v", st.stepEvent, want)
	}
	if got := stepNamesAndStatus(st.steps); !reflect.DeepEqual(got, []string{"archive=succeeded", "record=succeeded"}) {
		t.Fatalf("steps = %v", got)
	}
	if st.jobStatus != "succeeded" {
		t.Fatalf("job status = %q", st.jobStatus)
	}
}

func TestSFTPBackupStepTransitions(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	st.settings = sftpSettings()
	local := BackupRoot + "/zx_shop/x.tar.gz"
	h.outputs["backup.create"] = local + " 4096"
	site := store.Site{ID: "s1", NodeID: "n1", LinuxUser: "zx_shop", OwnerID: "u1", State: "ready"}
	jobID, err := m.createBackupJob(context.Background(), "u1", site)
	if err != nil {
		t.Fatal(err)
	}

	m.runBackup(context.Background(), site, jobID, "u1")

	want := []string{
		"archive:running", "archive:succeeded",
		"upload:running", "upload:succeeded",
		"record:running", "record:succeeded",
	}
	if !reflect.DeepEqual(st.stepEvent, want) {
		t.Fatalf("step events = %v, want %v", st.stepEvent, want)
	}
	if st.jobStatus != "succeeded" {
		t.Fatalf("job status = %q", st.jobStatus)
	}
}

func TestFailedUploadFailsStepAndSkipsRecord(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	st.settings = sftpSettings()
	h.outputs["backup.create"] = BackupRoot + "/zx_shop/x.tar.gz 4096"
	h.fail["backup.upload"] = errors.New("connection refused")
	site := store.Site{ID: "s1", NodeID: "n1", LinuxUser: "zx_shop", OwnerID: "u1", State: "ready"}
	jobID, err := m.createBackupJob(context.Background(), "u1", site)
	if err != nil {
		t.Fatal(err)
	}

	m.runBackup(context.Background(), site, jobID, "u1")

	want := []string{"archive=succeeded", "upload=failed", "record=pending"}
	if got := stepNamesAndStatus(st.steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	if st.steps[1].Error != "connection refused" {
		t.Fatalf("upload step error = %q", st.steps[1].Error)
	}
	if st.jobStatus != "failed" || st.jobErr != "connection refused" {
		t.Fatalf("job = %q / %q", st.jobStatus, st.jobErr)
	}
}

func TestFailedArchiveFailsFirstStep(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	h.fail["backup.create"] = errors.New("database dump failed")
	site := store.Site{ID: "s1", NodeID: "n1", LinuxUser: "zx_shop", OwnerID: "u1", State: "ready"}
	jobID, err := m.createBackupJob(context.Background(), "u1", site)
	if err != nil {
		t.Fatal(err)
	}

	m.runBackup(context.Background(), site, jobID, "u1")

	if got := stepNamesAndStatus(st.steps); !reflect.DeepEqual(got, []string{"archive=failed", "record=pending"}) {
		t.Fatalf("steps = %v", got)
	}
	if st.jobStatus != "failed" {
		t.Fatalf("job status = %q", st.jobStatus)
	}
}

func TestFailedRecordFailsRecordStep(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	st.insertErr = errors.New("insert backups: disk full")
	h.outputs["backup.create"] = BackupRoot + "/zx_shop/x.tar.gz 2048"
	site := store.Site{ID: "s1", NodeID: "n1", LinuxUser: "zx_shop", OwnerID: "u1", State: "ready"}
	jobID, err := m.createBackupJob(context.Background(), "u1", site)
	if err != nil {
		t.Fatal(err)
	}

	m.runBackup(context.Background(), site, jobID, "u1")

	if got := stepNamesAndStatus(st.steps); !reflect.DeepEqual(got, []string{"archive=succeeded", "record=failed"}) {
		t.Fatalf("steps = %v", got)
	}
	if st.jobStatus != "failed" {
		t.Fatalf("job status = %q", st.jobStatus)
	}
}

func TestStepErrorsNeverContainSFTPPassword(t *testing.T) {
	m, st, h := newFeatureTest("ready")
	st.settings = sftpSettings()
	st.settings.Destination.SFTP = passwordDest()
	withSFTPPassword(m, testPassword)
	h.outputs["backup.create"] = BackupRoot + "/zx_shop/x.tar.gz 4096"
	h.fail["backup.upload"] = errors.New("sftp auth rejected for password " + testPassword)
	site := store.Site{ID: "s1", NodeID: "n1", LinuxUser: "zx_shop", OwnerID: "u1", State: "ready"}
	jobID, err := m.createBackupJob(context.Background(), "u1", site)
	if err != nil {
		t.Fatal(err)
	}

	m.runBackup(context.Background(), site, jobID, "u1")

	for _, s := range st.steps {
		if strings.Contains(s.Error, testPassword) {
			t.Fatalf("step %s error leaks the password: %q", s.Name, s.Error)
		}
	}
	if !strings.Contains(st.steps[1].Error, "[redacted]") {
		t.Fatalf("upload step error not redacted: %q", st.steps[1].Error)
	}
	if strings.Contains(st.jobErr, testPassword) {
		t.Fatalf("job error leaks the password: %q", st.jobErr)
	}
}

func TestProgressPercent(t *testing.T) {
	cases := []struct {
		name     string
		statuses []string
		want     int
	}{
		{"no steps", nil, 0},
		{"all pending", []string{"pending", "pending", "pending"}, 0},
		{"first running", []string{"running", "pending"}, 25},
		{"one of two done", []string{"succeeded", "pending"}, 50},
		{"done and running of three", []string{"succeeded", "running", "pending"}, 50},
		{"two of three done", []string{"succeeded", "succeeded", "pending"}, 66},
		{"all done", []string{"succeeded", "succeeded", "succeeded"}, 100},
		{"failed counts nothing", []string{"succeeded", "failed"}, 50},
		{"single running", []string{"running"}, 50},
	}
	for _, c := range cases {
		if got := ProgressPercent(c.statuses); got != c.want {
			t.Errorf("%s: ProgressPercent(%v) = %d, want %d", c.name, c.statuses, got, c.want)
		}
	}
}
