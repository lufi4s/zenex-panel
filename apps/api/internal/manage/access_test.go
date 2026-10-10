package manage

import (
	"context"
	"fmt"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

func newAccessTest(state string) (*Manager, *featureStore, *scriptedHelper) {
	m, fs, h := newFeatureTest(state)
	m.SFTPPasswordFor = func(siteID string, gen int) string {
		return fmt.Sprintf("%s%02d", "abcdef0123456789abcdef", gen)
	}
	return m, fs, h
}

func TestEnableSFTPSendsDerivedPasswordToHelper(t *testing.T) {
	m, fs, h := newAccessTest("ready")
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", State: "ready"}
	if err := m.SetSFTP(context.Background(), site, true, false); err != nil {
		t.Fatal(err)
	}
	args := h.args["sftp.enable"][0]
	if args["user"] != "zx_shop" || args["password"] != "abcdef0123456789abcdef00" {
		t.Fatalf("sftp.enable args = %v", args)
	}
	if got := fs.sftp["s1"]; !got.enabled || got.generation != 0 {
		t.Fatalf("stored SFTP state = %+v", got)
	}
}

func TestResetSFTPPasswordUsesNextGeneration(t *testing.T) {
	m, fs, h := newAccessTest("ready")
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", State: "ready", SFTPEnabled: true, SFTPGeneration: 2}
	if err := m.SetSFTP(context.Background(), site, true, true); err != nil {
		t.Fatal(err)
	}
	if got := h.args["sftp.enable"][0]["password"]; got != "abcdef0123456789abcdef03" {
		t.Fatalf("rotated password = %q, want generation 3", got)
	}
	if got := fs.sftp["s1"]; got.generation != 3 || !got.enabled {
		t.Fatalf("stored SFTP state = %+v", got)
	}
}

func TestDisableSFTPLocksAccountThroughHelper(t *testing.T) {
	m, fs, h := newAccessTest("ready")
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", State: "ready", SFTPEnabled: true}
	if err := m.SetSFTP(context.Background(), site, false, false); err != nil {
		t.Fatal(err)
	}
	if len(h.args["sftp.disable"]) != 1 || h.args["sftp.disable"][0]["user"] != "zx_shop" {
		t.Fatalf("sftp.disable calls = %v", h.args["sftp.disable"])
	}
	if fs.sftp["s1"].enabled {
		t.Fatal("SFTP still marked enabled")
	}
}

func TestSFTPRefusesWebsitesThatAreNotReady(t *testing.T) {
	m, _, h := newAccessTest("suspended")
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", State: "suspended"}
	if err := m.SetSFTP(context.Background(), site, true, false); !asRefusal(err) {
		t.Fatalf("err = %v, want refusal", err)
	}
	if len(h.calls) != 0 {
		t.Fatalf("helper ran for a suspended website: %v", h.calls)
	}
}

func TestPrepareDBAdminRewritesPoolThenSiteFile(t *testing.T) {
	m, _, h := newAccessTest("ready")
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", Domain: "shop.example.com", PHPVersion: "8.3", State: "ready"}
	if err := m.PrepareDBAdmin(context.Background(), site); err != nil {
		t.Fatal(err)
	}
	if len(h.calls) != 2 || h.calls[0] != "pool.write" || h.calls[1] != "vhost.write" {
		t.Fatalf("calls = %v, want pool.write then vhost.write", h.calls)
	}
	if h.args["pool.write"][0]["php"] != "8.3" || h.args["vhost.write"][0]["maintenance"] != "0" {
		t.Fatalf("args = %v / %v", h.args["pool.write"], h.args["vhost.write"])
	}
}

func TestPrepareDBAdminRefusesMaintenanceMode(t *testing.T) {
	m, _, h := newAccessTest("ready")
	site := store.Site{ID: "s1", LinuxUser: "zx_shop", State: "ready", Maintenance: true}
	if err := m.PrepareDBAdmin(context.Background(), site); !asRefusal(err) {
		t.Fatalf("err = %v, want refusal", err)
	}
	if len(h.calls) != 0 {
		t.Fatalf("helper ran during maintenance: %v", h.calls)
	}
}
