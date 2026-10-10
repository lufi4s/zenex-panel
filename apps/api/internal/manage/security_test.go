package manage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

var secSite = store.Site{ID: "s1", LinuxUser: "zx_shop", Domain: "shop.example.com", State: "ready"}

// gatedHelper holds a scan until the test lets it go.
type gatedHelper struct {
	*scriptedHelper
	release chan struct{}
	started chan struct{}
}

func (g *gatedHelper) Do(ctx context.Context, op string, args map[string]string) (string, error) {
	if op == "malware.scan" {
		g.started <- struct{}{}
		<-g.release
	}
	return g.scriptedHelper.Do(ctx, op, args)
}

func waitIdle(t *testing.T, m *Manager, site store.Site) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if st, _ := m.SecurityStatus(context.Background(), site); !st.Running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the scan never finished")
}

func TestScanRunsInTheBackgroundOncePerWebsite(t *testing.T) {
	m, _, sh := newFeatureTest("ready")
	g := &gatedHelper{scriptedHelper: sh, release: make(chan struct{}), started: make(chan struct{}, 4)}
	m.Helper = g

	if err := m.StartScan(context.Background(), secSite); err != nil {
		t.Fatal(err)
	}
	<-g.started
	st, err := m.SecurityStatus(context.Background(), secSite)
	if err != nil || !st.Running || st.StartedAt == nil {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if err := m.StartScan(context.Background(), secSite); !asRefusal(err) {
		t.Fatalf("a second scan was allowed: %v", err)
	}
	other := secSite
	other.ID = "s2"
	other.LinuxUser = "zx_other"
	if err := m.StartScan(context.Background(), other); err != nil {
		t.Fatalf("scan of another website: %v", err)
	}
	close(g.release)
	waitIdle(t, m, secSite)
	waitIdle(t, m, other)
	if err := m.StartScan(context.Background(), secSite); err != nil {
		t.Fatalf("a finished scan blocks the next: %v", err)
	}
	waitIdle(t, m, secSite)
}

func TestFailedScanIsReportedWithoutInternals(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	h.fail["malware.scan"] = errors.New("exec: /var/www/zx_shop leaked detail")
	if err := m.StartScan(context.Background(), secSite); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, secSite)
	st, err := m.SecurityStatus(context.Background(), secSite)
	if err != nil || st.Running {
		t.Fatalf("status = %+v, %v", st, err)
	}
	if st.LastError == "" || strings.Contains(st.LastError, "leaked") {
		t.Fatalf("last error = %q", st.LastError)
	}
	h.fail = map[string]error{}
	if err := m.StartScan(context.Background(), secSite); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m, secSite)
	if st, _ := m.SecurityStatus(context.Background(), secSite); st.LastError != "" {
		t.Fatalf("stale error: %q", st.LastError)
	}
}

func TestSecurityStatusReadsReportAndQuarantine(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	h.outputs["malware.report"] = `{"scanned_at":"2026-01-02T03:04:05Z","files_scanned":12,"findings":[{"path":"a.php","rule":"eval-decode","severity":"high","message":"m","source":"signature"}]}`
	h.outputs["malware.list"] = `[{"id":"` + strings.Repeat("a", 24) + `","path":"b.php","size":3,"sha256":"x","quarantined_at":"2026-01-02T03:04:05Z"}]`
	st, err := m.SecurityStatus(context.Background(), secSite)
	if err != nil {
		t.Fatal(err)
	}
	if st.Report == nil || st.Report.FilesScanned != 12 || len(st.Report.Findings) != 1 || st.Report.Findings[0].Severity != "high" {
		t.Fatalf("report = %+v", st.Report)
	}
	if len(st.Quarantine) != 1 || st.Quarantine[0].Path != "b.php" {
		t.Fatalf("quarantine = %+v", st.Quarantine)
	}
}

func TestSecurityStatusBeforeFirstScanAndWithGarbage(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	st, err := m.SecurityStatus(context.Background(), secSite)
	if err != nil || st.Report != nil || st.Quarantine == nil || len(st.Quarantine) != 0 {
		t.Fatalf("status = %+v, %v", st, err)
	}
	h.outputs["malware.report"] = "not json"
	h.outputs["malware.list"] = "null"
	st, err = m.SecurityStatus(context.Background(), secSite)
	if err != nil || st.Report != nil || st.Quarantine == nil {
		t.Fatalf("garbage status = %+v, %v", st, err)
	}
}

func TestQuarantineActionsPassArguments(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	ctx := context.Background()
	id := strings.Repeat("b", 24)
	if err := m.Quarantine(ctx, secSite, "wp-content/uploads/x.php", "request-exec"); err != nil {
		t.Fatal(err)
	}
	if err := m.RestoreQuarantined(ctx, secSite, id); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteQuarantined(ctx, secSite, id); err != nil {
		t.Fatal(err)
	}
	q := h.args["malware.quarantine"][0]
	if q["user"] != "zx_shop" || q["path"] != "wp-content/uploads/x.php" || q["rule"] != "request-exec" {
		t.Fatalf("quarantine args = %v", q)
	}
	if h.args["malware.restore"][0]["id"] != id || h.args["malware.purge"][0]["id"] != id {
		t.Fatal("item id was not passed")
	}
	if err := m.Quarantine(ctx, secSite, "", ""); err == nil {
		t.Fatal("an empty path was accepted")
	}
}

func TestScannerRefusesWebsitesThatAreNotReady(t *testing.T) {
	m, _, h := newFeatureTest("provisioning")
	site := secSite
	site.State = "provisioning"
	ctx := context.Background()
	if err := m.StartScan(ctx, site); !asRefusal(err) {
		t.Errorf("scan: %v", err)
	}
	if _, err := m.SecurityStatus(ctx, site); !asRefusal(err) {
		t.Errorf("status: %v", err)
	}
	if err := m.Quarantine(ctx, site, "a.php", ""); !asRefusal(err) {
		t.Errorf("quarantine: %v", err)
	}
	if len(h.args) != 0 {
		t.Fatalf("helper ran for a refused action: %v", h.args)
	}
}
