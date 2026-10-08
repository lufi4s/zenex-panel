package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// SetSitePHPVersion records the PHP version a site runs on.
func (s *Store) SetSitePHPVersion(ctx context.Context, siteID, version string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sites SET php_version = $2, updated_at = now() WHERE id = $1::uuid`, siteID, version)
	return err
}

// MarkSiteDeleted hides a site after its files, database and account are gone.
func (s *Store) MarkSiteDeleted(ctx context.Context, siteID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Remove the address so it can be used again; the site row stays for history.
	if _, err := tx.Exec(ctx, `DELETE FROM domains WHERE site_id = $1::uuid`, siteID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sites SET state = 'deleted', deleted_at = now(), updated_at = now()
		WHERE id = $1::uuid`, siteID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CreateManagementJob records a site operation (for example deletion) as a running job.
func (s *Store) CreateManagementJob(ctx context.Context, actorID, siteID, nodeID, jobType string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO jobs (type, status, actor_user_id, node_id, site_id, started_at, attempts)
		VALUES ($1, 'running', $2::uuid, NULLIF($3, '')::uuid, $4::uuid, now(), 1)
		RETURNING id::text`, jobType, actorID, nodeID, siteID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}
