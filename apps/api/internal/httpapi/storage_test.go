package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

func TestStorageListsEachWebsiteLargestFirst(t *testing.T) {
	e := newSettingsEnv(t)
	e.sites.sites = []store.Site{
		{ID: "11111111-aaaa-4aaa-8aaa-111111111111", Domain: "shop.example.com", LinuxUser: "zx_shop", OwnerID: "11111111-1111-1111-1111-111111111111"},
		{ID: "22222222-aaaa-4aaa-8aaa-222222222222", Domain: "blog.example.com", LinuxUser: "zx_blog", OwnerID: "11111111-1111-1111-1111-111111111111"},
	}
	admin := e.login(t, "admin@example.com")
	rec := e.call(t, http.MethodGet, "/api/v1/storage", "", admin, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	var got storageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Sites) != 2 {
		t.Fatalf("sites = %+v", got.Sites)
	}
	// zx_shop has usage from the fake manager (3000 + 2000 + 1000); zx_blog has none, so 0.
	first := got.Sites[0]
	if first.Domain != "shop.example.com" || first.FilesBytes != 3000 || first.DatabaseBytes != 2000 ||
		first.BackupBytes != 1000 || first.TotalBytes != 6000 {
		t.Errorf("shop row = %+v", first)
	}
	if got.Sites[1].TotalBytes != 0 {
		t.Errorf("site without usage = %+v", got.Sites[1])
	}
}

func TestStorageNeedsSignInAndHidesServerDiskFromCustomers(t *testing.T) {
	e := newSettingsEnv(t)
	if rec := e.call(t, http.MethodGet, "/api/v1/storage", "", nil, false); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want 401", rec.Code)
	}
	customer := e.login(t, "customer@example.com")
	rec := e.call(t, http.MethodGet, "/api/v1/storage", "", customer, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("customer status = %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["disk"]; ok {
		t.Fatal("customer sees the server disk")
	}
}
