package manage

import (
	"context"
	"testing"
)

func TestSiteUsagesReadsTheHelperAndCachesIt(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	h.outputs["usage.sites"] = `[{"user":"zx_shop","files_bytes":150,"database_bytes":4096,"backup_bytes":700}]`

	for i := 0; i < 2; i++ {
		usages, err := m.SiteUsages(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got := usages["zx_shop"]; got.FilesBytes != 150 || got.DatabaseBytes != 4096 || got.BackupBytes != 700 {
			t.Fatalf("usage = %+v", got)
		}
	}
	calls := 0
	for _, op := range h.calls {
		if op == "usage.sites" {
			calls++
		}
	}
	if calls != 1 {
		t.Fatalf("helper measured %d times, want once (second answer comes from the cache)", calls)
	}
}

func TestSiteUsagesRejectsAnInvalidAnswer(t *testing.T) {
	m, _, h := newFeatureTest("ready")
	h.outputs["usage.sites"] = "not json"
	if _, err := m.SiteUsages(context.Background()); err == nil {
		t.Fatal("invalid helper answer accepted")
	}
}
