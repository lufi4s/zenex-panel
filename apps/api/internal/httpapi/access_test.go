package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/provision"
)

const accessSiteID = "11111111-aaaa-4aaa-8aaa-111111111111"

func TestSiteAccessGivesOwnerTheDatabaseLoginWithoutSFTPPassword(t *testing.T) {
	e := newSettingsEnv(t)
	owner := e.login(t, "admin@example.com")
	rec := e.call(t, http.MethodGet, "/api/v1/sites/"+accessSiteID+"/access", "", owner, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	var got siteAccess
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Database.Password != provision.DBPassword([]byte(testServerSecret), accessSiteID) {
		t.Fatalf("database password does not match the site's derived password")
	}
	if got.Database.Host != "localhost" || got.PhpMyAdmin.URL != "https://shop.example.com/zenex-dbadmin/" {
		t.Fatalf("database host or phpMyAdmin address = %q / %q", got.Database.Host, got.PhpMyAdmin.URL)
	}
	if got.SFTP.Enabled || got.SFTP.Password != "" || got.SFTP.Port != 22 {
		t.Fatalf("SFTP should be off with no password shown: %+v", got.SFTP)
	}
}

func TestSiteAccessIsHiddenFromOtherCustomers(t *testing.T) {
	e := newSettingsEnv(t)
	customer := e.login(t, "customer@example.com")
	if rec := e.call(t, http.MethodGet, "/api/v1/sites/"+accessSiteID+"/access", "", customer, false); rec.Code != http.StatusNotFound {
		t.Fatalf("another customer's site access = %d, want 404", rec.Code)
	}
}

func TestSFTPChangesNeedCSRFAndReachTheManager(t *testing.T) {
	e := newSettingsEnv(t)
	owner := e.login(t, "admin@example.com")
	path := "/api/v1/sites/" + accessSiteID + "/access/sftp"

	if rec := e.call(t, http.MethodPut, path, "", owner, false); rec.Code != http.StatusForbidden {
		t.Fatalf("enable without CSRF = %d, want 403", rec.Code)
	}
	if rec := e.call(t, http.MethodPut, path, "", owner, true); rec.Code != http.StatusOK {
		t.Fatalf("enable = %d body %s", rec.Code, rec.Body)
	}
	if rec := e.call(t, http.MethodPut, path+"?reset=1", "", owner, true); rec.Code != http.StatusOK {
		t.Fatalf("reset = %d body %s", rec.Code, rec.Body)
	}
	if rec := e.call(t, http.MethodDelete, path, "", owner, true); rec.Code != http.StatusOK {
		t.Fatalf("disable = %d body %s", rec.Code, rec.Body)
	}
	want := []string{"enable", "enable-reset", "disable"}
	if len(e.manage.sftp) != len(want) {
		t.Fatalf("manager calls = %v, want %v", e.manage.sftp, want)
	}
	for i := range want {
		if e.manage.sftp[i] != want[i] {
			t.Fatalf("manager calls = %v, want %v", e.manage.sftp, want)
		}
	}
}

func TestOpenPhpMyAdminReturnsTheSiteAddress(t *testing.T) {
	e := newSettingsEnv(t)
	owner := e.login(t, "admin@example.com")
	rec := e.call(t, http.MethodPost, "/api/v1/sites/"+accessSiteID+"/access/phpmyadmin", "", owner, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body)
	}
	var got phpMyAdminAccess
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://shop.example.com/zenex-dbadmin/" || e.manage.dbadmin != 1 {
		t.Fatalf("url = %q, preparations = %d", got.URL, e.manage.dbadmin)
	}
}
