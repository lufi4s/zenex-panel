package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// SiteDefaults are the values new websites start with.
type SiteDefaults struct {
	PHPVersion string `json:"php_version"`
}

// Backup destination types.
const (
	BackupDestLocal = "local"
	BackupDestSFTP  = "sftp"
)

// BackupSettings control the daily backup run, how long backups are kept and where
// they are sent. A stored value without a destination means local storage.
type BackupSettings struct {
	ScheduleHour  int               `json:"schedule_hour"`
	RetentionDays int               `json:"retention_days"`
	Destination   BackupDestination `json:"destination"`
}

// BackupDestination is where archives are kept: "local" (this server) or "sftp".
type BackupDestination struct {
	Type string          `json:"type"`
	SFTP SFTPDestination `json:"sftp"`
}

// SFTPDestination is a remote SFTP server. The panel's backup key must be installed
// for Username on that server (see POST /api/v1/settings/backups/sftp-key).
type SFTPDestination struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Path     string `json:"path"`
}

// DefaultBackupSettings is used until an administrator changes them.
func DefaultBackupSettings() BackupSettings {
	return BackupSettings{
		ScheduleHour:  3,
		RetentionDays: 7,
		Destination: BackupDestination{
			Type: BackupDestLocal,
			SFTP: SFTPDestination{Port: 22, Path: "/backups/zenex"},
		},
	}
}

// GetSetting returns the raw JSON stored under key, or ErrNotFound.
func (s *Store) GetSetting(ctx context.Context, key string) (json.RawMessage, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value::text FROM system_settings WHERE key = $1`, key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// PutSetting stores value (JSON-encoded) under key and records who changed it.
func (s *Store) PutSetting(ctx context.Context, key string, value any, userID string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO system_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2::jsonb, NULLIF($3, '')::uuid, now())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		key, string(raw), userID)
	return err
}

// GetSiteDefaults returns the saved defaults. A zero value means none are saved yet.
func (s *Store) GetSiteDefaults(ctx context.Context) (SiteDefaults, error) {
	var d SiteDefaults
	raw, err := s.GetSetting(ctx, "site_defaults")
	if errors.Is(err, ErrNotFound) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	return d, json.Unmarshal(raw, &d)
}

// SetSiteDefaults saves the defaults for new websites.
func (s *Store) SetSiteDefaults(ctx context.Context, userID string, d SiteDefaults) error {
	return s.PutSetting(ctx, "site_defaults", d, userID)
}

// GetBackupSettings returns the saved backup settings, or the defaults.
func (s *Store) GetBackupSettings(ctx context.Context) (BackupSettings, error) {
	b := DefaultBackupSettings()
	raw, err := s.GetSetting(ctx, "backup_settings")
	if errors.Is(err, ErrNotFound) {
		return b, nil
	}
	if err != nil {
		return b, err
	}
	return decodeBackupSettings(raw)
}

// decodeBackupSettings reads a stored backup_settings value. Missing keys keep their
// defaults, and a value saved before SFTP support (no destination) means local storage.
func decodeBackupSettings(raw []byte) (BackupSettings, error) {
	b := DefaultBackupSettings()
	if err := json.Unmarshal(raw, &b); err != nil {
		return b, err
	}
	if b.Destination.Type == "" {
		b.Destination.Type = BackupDestLocal
	}
	return b, nil
}

// SetBackupSettings saves the backup schedule and retention.
func (s *Store) SetBackupSettings(ctx context.Context, userID string, b BackupSettings) error {
	return s.PutSetting(ctx, "backup_settings", b, userID)
}

// AdministratorIDs lists the accounts that receive operator notifications.
func (s *Store) AdministratorIDs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ur.user_id::text FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		JOIN users u ON u.id = ur.user_id
		WHERE r.name = 'administrator' AND u.status = 'active'
		ORDER BY ur.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// sftpPublicKeySetting holds the public half of the backup key. It is not secret.
const sftpPublicKeySetting = "backup_sftp_public_key"

// GetSFTPPublicKey returns the saved backup public key, or ErrNotFound when none was created.
func (s *Store) GetSFTPPublicKey(ctx context.Context) (string, error) {
	raw, err := s.GetSetting(ctx, sftpPublicKeySetting)
	if err != nil {
		return "", err
	}
	var key string
	if err := json.Unmarshal(raw, &key); err != nil {
		return "", err
	}
	return key, nil
}

// SetSFTPPublicKey records the public key the helper generated.
func (s *Store) SetSFTPPublicKey(ctx context.Context, userID, key string) error {
	return s.PutSetting(ctx, sftpPublicKeySetting, key, userID)
}
