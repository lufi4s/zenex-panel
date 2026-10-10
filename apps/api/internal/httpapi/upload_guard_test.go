package httpapi

import (
	"net/http"
	"testing"
)

const uploadPath = "/api/v1/sites/11111111-aaaa-4aaa-8aaa-111111111111/files/upload?path=/"

// The upload limits are checked before any bytes are read, so a full disk or a busy server
// answers at once instead of accepting a 64 MB file it cannot store.
func TestUploadRefusedWhenDiskIsNearlyFull(t *testing.T) {
	old := diskFree
	t.Cleanup(func() { diskFree = old })
	diskFree = func(string) (uint64, error) { return minFreeForUpload - 1, nil }

	e := newSettingsEnv(t)
	owner := e.login(t, "admin@example.com")
	rec := e.call(t, http.MethodPost, uploadPath, "", owner, true)
	if rec.Code != http.StatusInsufficientStorage {
		t.Fatalf("upload with a full disk = %d, want 507", rec.Code)
	}
}

func TestUploadRefusedWhenTooManyAreRunning(t *testing.T) {
	for i := 0; i < maxConcurrentUploads; i++ {
		uploadSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for i := 0; i < maxConcurrentUploads; i++ {
			<-uploadSlots
		}
	})

	e := newSettingsEnv(t)
	owner := e.login(t, "admin@example.com")
	rec := e.call(t, http.MethodPost, uploadPath, "", owner, true)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("upload with all slots busy = %d, want 503", rec.Code)
	}
}
