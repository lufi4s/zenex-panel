package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBackupSettingsWithoutDestinationMeansLocal(t *testing.T) {
	b, err := decodeBackupSettings([]byte(`{"schedule_hour":2,"retention_days":30}`))
	if err != nil {
		t.Fatal(err)
	}
	if b.ScheduleHour != 2 || b.RetentionDays != 30 || b.Destination.Type != BackupDestLocal {
		t.Fatalf("legacy value decoded as %+v", b)
	}
}

func TestBackupSettingsSFTPDestinationRoundTrip(t *testing.T) {
	raw := []byte(`{"schedule_hour":3,"retention_days":7,"destination":{"type":"sftp","sftp":{"host":"backup.example.com","port":2222,"username":"zx_backup","path":"/backups/zenex"}}}`)
	b, err := decodeBackupSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := SFTPDestination{Host: "backup.example.com", Port: 2222, Username: "zx_backup", Path: "/backups/zenex", Auth: SFTPAuthKey}
	if b.Destination.Type != BackupDestSFTP || b.Destination.SFTP != want {
		t.Fatalf("decoded %+v", b.Destination)
	}
}

func TestBackupSettingsRejectGarbage(t *testing.T) {
	if _, err := decodeBackupSettings([]byte(`{"schedule_hour":"three"}`)); err == nil {
		t.Fatal("type mismatch accepted")
	}
}

func TestBackupSettingsWithoutAuthMeansKey(t *testing.T) {
	b, err := decodeBackupSettings([]byte(`{"destination":{"type":"sftp","sftp":{"host":"h.example","port":22}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if b.Destination.SFTP.Auth != SFTPAuthKey {
		t.Fatalf("auth = %q, want key", b.Destination.SFTP.Auth)
	}
	raw, err := json.Marshal(DefaultBackupSettings())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "password") {
		t.Fatalf("backup settings document mentions a password: %s", raw)
	}
}
