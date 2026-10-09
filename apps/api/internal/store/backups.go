package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
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

// SitesWithActiveBackup returns the IDs of sites that have a backup or restore job queued
// or running. Both change the same files and database, so neither may overlap another.
func (s *Store) SitesWithActiveBackup(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT site_id::text FROM jobs
		WHERE type IN ('site.backup', 'site.restore', 'site.migrate') AND status IN ('queued', 'running') AND site_id IS NOT NULL`)
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

// GetBackup returns one backup of a site, or ErrNotFound when it does not belong to that site.
func (s *Store) GetBackup(ctx context.Context, siteID string, id int64) (Backup, error) {
	var b Backup
	err := s.pool.QueryRow(ctx, `
		SELECT id, site_id::text, created_at, size_bytes, path FROM backups
		WHERE id = $1 AND site_id = $2::uuid`, id, siteID).
		Scan(&b.ID, &b.SiteID, &b.CreatedAt, &b.SizeBytes, &b.Path)
	if errors.Is(err, pgx.ErrNoRows) {
		return Backup{}, ErrNotFound
	}
	return b, err
}
