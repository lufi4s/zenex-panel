package store

import (
	"context"
	"time"
)

// Backup is one archive of a website (files and database).
// The path is a server-side location and is never sent to customers.
type Backup struct {
	ID        int64     `json:"id"`
	SiteID    string    `json:"site_id"`
	CreatedAt time.Time `json:"created_at"`
	SizeBytes int64     `json:"size_bytes"`
	Path      string    `json:"-"`
}

// InsertBackup records a finished backup.
func (s *Store) InsertBackup(ctx context.Context, siteID string, sizeBytes int64, path string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO backups (site_id, size_bytes, path) VALUES ($1::uuid, $2, $3)`,
		siteID, sizeBytes, path)
	return err
}

// ListBackups returns a site's backups, newest first.
func (s *Store) ListBackups(ctx context.Context, siteID string) ([]Backup, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, site_id::text, created_at, size_bytes, path FROM backups
		WHERE site_id = $1::uuid ORDER BY created_at DESC, id DESC`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Backup{}
	for rows.Next() {
		var b Backup
		if err := rows.Scan(&b.ID, &b.SiteID, &b.CreatedAt, &b.SizeBytes, &b.Path); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BackupsBefore returns backups created before cutoff, oldest first.
func (s *Store) BackupsBefore(ctx context.Context, cutoff time.Time) ([]Backup, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, site_id::text, created_at, size_bytes, path FROM backups
		WHERE created_at < $1 ORDER BY created_at, id`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Backup
	for rows.Next() {
		var b Backup
		if err := rows.Scan(&b.ID, &b.SiteID, &b.CreatedAt, &b.SizeBytes, &b.Path); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteBackup removes a backup record once its archive is gone.
func (s *Store) DeleteBackup(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM backups WHERE id = $1`, id)
	return err
}
