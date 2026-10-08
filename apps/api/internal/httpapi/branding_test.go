package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

func TestBrandingValidation(t *testing.T) {
	ok := store.Branding{Name: "  My Hosting  ", Tagline: "Fast sites", PrimaryColor: "#A1B2C3"}
	got, problem := validBranding(ok)
	if problem != "" || got.Name != "My Hosting" || got.PrimaryColor != "#a1b2c3" {
		t.Fatalf("valid branding rejected or not normalised: %+v %q", got, problem)
	}
	bad := []store.Branding{
		{Name: "A", PrimaryColor: "#000000"},
		{Name: "Ok name", PrimaryColor: "red"},
		{Name: "Ok name", PrimaryColor: "#12345"},
		{Name: "Ok name", Tagline: "line\nbreak", PrimaryColor: "#000000"},
	}
	for _, b := range bad {
		if _, problem := validBranding(b); problem == "" {
			t.Errorf("invalid branding accepted: %+v", b)
		}
	}
}

// The sign-in page needs the name before anyone is signed in.
func TestBrandingIsPublicAndHasDefaults(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/branding", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var b store.Branding
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil || b.Name == "" || b.PrimaryColor == "" {
		t.Fatalf("defaults missing: %s", rec.Body.String())
	}
}

func TestBrandingChangeRequiresSignIn(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/branding", nil)
	req.Header.Set("X-Requested-With", "zenex")
	newTestRouter(nil).ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || rec.Code == http.StatusNoContent {
		t.Fatalf("change accepted without a session: status %d", rec.Code)
	}
}
