package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/alerts"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/helperclient"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

func sftpBackupBody(host, user, path string, port int) string {
	return `{"schedule_hour":3,"retention_days":7,"destination":{"type":"sftp","sftp":{"host":"` + host +
		`","port":` + strconv.Itoa(port) + `,"username":"` + user + `","path":"` + path + `"}}}`
}

func TestBackupDestinationValidation(t *testing.T) {
	good := []store.BackupDestination{
		{Type: "local"},
		{Type: "sftp", SFTP: store.SFTPDestination{Host: "backup.example.com", Port: 22, Username: "zx_backup", Path: "/backups/zenex/"}},
		{Type: "sftp", SFTP: store.SFTPDestination{Host: "203.0.113.10", Port: 65535, Username: "_b", Path: "/srv/x-y_z"}},
		{Type: "sftp", SFTP: store.SFTPDestination{Host: "2001:db8::1", Port: 1, Username: "a", Path: "/b"}},
		{SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "u", Path: "/b"}},
	}
	for _, dest := range good {
		in := store.BackupSettings{ScheduleHour: 3, RetentionDays: 7, Destination: dest}
		if _, problem := validBackupSettings(in); problem != "" {
			t.Errorf("rejected %+v: %s", dest, problem)
		}
	}
	long := "/" + strings.Repeat("a", 200)
	bad := map[string]store.BackupDestination{
		"unknown type":       {Type: "ftp"},
		"host with space":    {Type: "sftp", SFTP: store.SFTPDestination{Host: "bad host", Port: 22, Username: "u", Path: "/b"}},
		"host with slash":    {Type: "sftp", SFTP: store.SFTPDestination{Host: "host/x", Port: 22, Username: "u", Path: "/b"}},
		"empty host":         {Type: "sftp", SFTP: store.SFTPDestination{Port: 22, Username: "u", Path: "/b"}},
		"port zero":          {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 0, Username: "u", Path: "/b"}},
		"port too big":       {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 65536, Username: "u", Path: "/b"}},
		"username upper":     {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "Root", Path: "/b"}},
		"username digit 1st": {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "1abc", Path: "/b"}},
		"username too long":  {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: strings.Repeat("a", 33), Path: "/b"}},
		"relative path":      {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "u", Path: "backups"}},
		"dot dot":            {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "u", Path: "/backups/../etc"}},
		"blank path":         {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "u", Path: "   "}},
		"root path":          {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "u", Path: "/"}},
		"path too long":      {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "u", Path: long}},
		"control char path":  {Type: "sftp", SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "u", Path: "/b\n/c"}},
	}
	for name, dest := range bad {
		in := store.BackupSettings{ScheduleHour: 3, RetentionDays: 7, Destination: dest}
		if _, problem := validBackupSettings(in); problem == "" {
			t.Errorf("%s: accepted %+v", name, dest)
		}
	}
}

func TestBackupDestinationTrimsAndCleansPath(t *testing.T) {
	in := store.BackupSettings{ScheduleHour: 3, RetentionDays: 7, Destination: store.BackupDestination{Type: "SFTP",
		SFTP: store.SFTPDestination{Host: " h.example ", Port: 22, Username: " zx_b ", Path: " /backups//zenex/ "}}}
	got, problem := validBackupSettings(in)
	if problem != "" || got.Destination.Type != "sftp" || got.Destination.SFTP.Host != "h.example" ||
		got.Destination.SFTP.Username != "zx_b" || got.Destination.SFTP.Path != "/backups/zenex" {
		t.Fatalf("normalised = %+v %q", got.Destination, problem)
	}
}

func TestBackupSFTPSettingsSaveAndReturnShape(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	rec := e.call(t, http.MethodPut, "/api/v1/settings/backups", sftpBackupBody("backup.example.com", "zx_backup", "/backups/zenex", 2222), admin, true)
	if rec.Code != http.StatusOK || e.sites.backup.Destination.SFTP.Port != 2222 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body.String())
	}
	var got store.BackupSettings
	rec = e.call(t, http.MethodGet, "/api/v1/settings/backups", "", admin, false)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Destination.Type != "sftp" || got.Destination.SFTP.Host != "backup.example.com" {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body.String())
	}
	rec = e.call(t, http.MethodPut, "/api/v1/settings/backups", sftpBackupBody("bad host", "zx_backup", "/backups", 22), admin, true)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_backup_settings" {
		t.Fatalf("bad host = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSFTPKeyEndpoints(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	customer := e.login(t, "customer@example.com")
	const path = "/api/v1/settings/backups/sftp-key"

	if rec := e.call(t, http.MethodGet, path, "", admin, false); rec.Code != http.StatusNotFound || errorCode(t, rec) != "key_missing" {
		t.Fatalf("GET before creation = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.call(t, http.MethodPost, path, "", admin, false); rec.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF = %d", rec.Code)
	}
	if rec := e.call(t, http.MethodPost, path, "", customer, true); rec.Code != http.StatusForbidden {
		t.Fatalf("POST as customer = %d", rec.Code)
	}
	rec := e.call(t, http.MethodPost, path, "", admin, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"public_key":"ssh-ed25519 AAAA`) {
		t.Fatalf("POST = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.call(t, http.MethodGet, path, "", admin, false); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "zenex-backup") {
		t.Fatalf("GET after creation = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.call(t, http.MethodGet, path, "", customer, false); rec.Code != http.StatusForbidden {
		t.Fatalf("GET as customer = %d", rec.Code)
	}
}

func TestSFTPTestEndpoint(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	const path = "/api/v1/settings/backups/sftp-test"

	if rec := e.call(t, http.MethodPost, path, "", admin, true); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "sftp_not_configured" {
		t.Fatalf("local destination = %d %s", rec.Code, rec.Body.String())
	}
	e.call(t, http.MethodPut, "/api/v1/settings/backups", sftpBackupBody("backup.example.com", "zx_backup", "/backups/zenex", 2222), admin, true)

	if rec := e.call(t, http.MethodPost, path, "", admin, false); rec.Code != http.StatusForbidden {
		t.Fatalf("without CSRF = %d", rec.Code)
	}
	if rec := e.call(t, http.MethodPost, path, "", admin, true); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("success = %d %s", rec.Code, rec.Body.String())
	}
	if len(e.manage.tested) != 1 || e.manage.tested[0].Host != "backup.example.com" || e.manage.tested[0].Port != 2222 {
		t.Fatalf("tested %+v", e.manage.tested)
	}

	e.manage.testErr = &helperclient.Error{Message: "Permission denied (publickey)"}
	rec := e.call(t, http.MethodPost, path, "", admin, true)
	if rec.Code != http.StatusBadGateway || errorCode(t, rec) != "sftp_failed" || !strings.Contains(rec.Body.String(), "Permission denied") {
		t.Fatalf("failure = %d %s", rec.Code, rec.Body.String())
	}
}

// sftpBackupAuthBody builds a PUT body with the given auth and password (both may be empty).
func sftpBackupAuthBody(auth, password string) string {
	authField := ""
	if auth != "" {
		authField = `,"auth":"` + auth + `"`
	}
	passField := ""
	if password != "" {
		passField = `,"password":"` + password + `"`
	}
	return `{"schedule_hour":3,"retention_days":7,"destination":{"type":"sftp","sftp":{"host":"backup.example.com","port":2222,"username":"zx_backup","path":"/backups/zenex"` +
		authField + passField + `}}}`
}

func TestSFTPAuthValues(t *testing.T) {
	for _, auth := range []string{"", "key", "KEY", "password"} {
		in := store.BackupSettings{ScheduleHour: 3, RetentionDays: 7, Destination: store.BackupDestination{Type: "sftp",
			SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "u", Path: "/b", Auth: auth}}}
		if _, problem := validBackupSettings(in); problem != "" && auth != "password" {
			t.Errorf("auth %q rejected: %s", auth, problem)
		}
	}
	in := store.BackupSettings{ScheduleHour: 3, RetentionDays: 7, Destination: store.BackupDestination{Type: "sftp",
		SFTP: store.SFTPDestination{Host: "h.example", Port: 22, Username: "u", Path: "/b", Auth: "ftp"}}}
	if _, problem := validBackupSettings(in); problem == "" {
		t.Fatal("auth ftp accepted")
	}
}

func TestSFTPAuthInvalidValueIs400(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	rec := e.call(t, http.MethodPut, "/api/v1/settings/backups", sftpBackupAuthBody("ftp", ""), admin, true)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_backup_settings" {
		t.Fatalf("auth ftp = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSFTPPasswordRequiredWhenAuthIsPassword(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	const path = "/api/v1/settings/backups"

	rec := e.call(t, http.MethodPut, path, sftpBackupAuthBody("password", ""), admin, true)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "sftp_password_required" {
		t.Fatalf("password auth without password = %d %s", rec.Code, rec.Body.String())
	}
	if e.sites.sealed != "" || e.sites.backup.Destination.SFTP.Auth == "password" {
		t.Fatal("refused save changed stored state")
	}

	rec = e.call(t, http.MethodPut, path, sftpBackupAuthBody("password", "s3cret-Value"), admin, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"password_set":true`) {
		t.Fatalf("save with password = %d %s", rec.Code, rec.Body.String())
	}

	// An empty password keeps the stored one.
	rec = e.call(t, http.MethodPut, path, sftpBackupAuthBody("password", ""), admin, true)
	if rec.Code != http.StatusOK || e.sites.sealed == "" {
		t.Fatalf("save keeping password = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSFTPPasswordIsEncryptedAndNotInBackupSettings(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	const password = "s3cret-Value"
	rec := e.call(t, http.MethodPut, "/api/v1/settings/backups", sftpBackupAuthBody("password", password), admin, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("save = %d %s", rec.Code, rec.Body.String())
	}

	settingsJSON, err := json.Marshal(e.sites.backup)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(settingsJSON), password) || strings.Contains(string(settingsJSON), `"password":`) {
		t.Fatalf("backup settings document holds the password: %s", settingsJSON)
	}
	if e.sites.sealed == "" || strings.Contains(e.sites.sealed, password) {
		t.Fatalf("stored value %q is not an encrypted password", e.sites.sealed)
	}
	plain, err := alerts.Open(alerts.DeriveKey(testServerSecret), e.sites.sealed)
	if err != nil || plain != password {
		t.Fatalf("decrypted = %q, %v", plain, err)
	}
}

func TestSFTPPasswordNeedsServerSecret(t *testing.T) {
	e := newSettingsEnvWithSecret(t, "")
	admin := e.login(t, testEmail)
	rec := e.call(t, http.MethodPut, "/api/v1/settings/backups", sftpBackupAuthBody("password", "s3cret-Value"), admin, true)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "secret_key_missing" {
		t.Fatalf("password without secret = %d %s", rec.Code, rec.Body.String())
	}
	if e.sites.sealed != "" {
		t.Fatal("password stored without a server secret")
	}
	if rec := e.call(t, http.MethodPut, "/api/v1/settings/backups", sftpBackupAuthBody("key", ""), admin, true); rec.Code != http.StatusOK {
		t.Fatalf("key auth without secret = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSFTPPasswordLengthAndControlCharactersRejected(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	bodies := []string{
		sftpBackupAuthBody("password", strings.Repeat("p", 513)),
		// JSON escape for a newline inside the password.
		strings.Replace(sftpBackupAuthBody("password", "x"), `"password":"x"`, `"password":"line\nbreak"`, 1),
	}
	for _, body := range bodies {
		rec := e.call(t, http.MethodPut, "/api/v1/settings/backups", body, admin, true)
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_backup_settings" {
			t.Errorf("body rejected wrongly: %d %s", rec.Code, rec.Body.String())
		}
	}
	if e.sites.sealed != "" {
		t.Fatal("rejected password stored")
	}
}

func TestGetBackupSettingsShowsPasswordSetNotValue(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, testEmail)
	const password = "s3cret-Value"
	rec := e.call(t, http.MethodGet, "/api/v1/settings/backups", "", admin, false)
	if !strings.Contains(rec.Body.String(), `"password_set":false`) || !strings.Contains(rec.Body.String(), `"auth":"key"`) {
		t.Fatalf("GET before save = %d %s", rec.Code, rec.Body.String())
	}

	e.call(t, http.MethodPut, "/api/v1/settings/backups", sftpBackupAuthBody("password", password), admin, true)
	rec = e.call(t, http.MethodGet, "/api/v1/settings/backups", "", admin, false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"password_set":true`) || !strings.Contains(rec.Body.String(), `"auth":"password"`) {
		t.Fatalf("GET after save = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), password) || strings.Contains(rec.Body.String(), e.sites.sealed) {
		t.Fatalf("GET leaks the password: %s", rec.Body.String())
	}
}
