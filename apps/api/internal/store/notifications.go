package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Notification is one message shown in the notification centre.
type Notification struct {
	ID        int64      `json:"id"`
	Level     string     `json:"level"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

// ErrDomainInUse means a domain still has live websites on it.
var ErrDomainInUse = errors.New("domain has live websites")

// Notify stores a message for one account. Titles and bodies are kept short so
// the notification centre stays readable.
func (s *Store) Notify(ctx context.Context, userID, level, title, body string) error {
	if userID == "" {
		return nil
	}
	if len(title) > 200 {
		title = title[:200]
	}
	if len(body) > 2000 {
		body = body[:2000]
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO notifications (user_id, level, title, body)
		VALUES ($1::uuid, $2, $3, $4)`, userID, level, title, body)
	return err
}

// ListNotifications returns the newest notifications for an account.
func (s *Store) ListNotifications(ctx context.Context, userID string, limit int) ([]Notification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, level, title, body, created_at, read_at FROM notifications
		WHERE user_id = $1::uuid ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Level, &n.Title, &n.Body, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CountUnread returns how many notifications the account has not read.
func (s *Store) CountUnread(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1::uuid AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

// MarkNotificationsRead marks the given notifications as read. With all set, every
// unread notification of the account is marked.
func (s *Store) MarkNotificationsRead(ctx context.Context, userID string, ids []int64, all bool) error {
	if all {
		_, err := s.pool.Exec(ctx, `
			UPDATE notifications SET read_at = now()
			WHERE user_id = $1::uuid AND read_at IS NULL`, userID)
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE user_id = $1::uuid AND read_at IS NULL AND id = ANY($2::bigint[])`, userID, ids)
	return err
}

// DeleteDomain removes a connected domain. It refuses while live websites use it,
// and reports how many.
func (s *Store) DeleteDomain(ctx context.Context, ownerID, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var owned bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM dns_zones WHERE id = $1::uuid AND owner_user_id = $2::uuid)`,
		id, ownerID).Scan(&owned); err != nil {
		return err
	}
	if !owned {
		return ErrNotFound
	}

	var live int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM sites WHERE zone_id = $1::uuid AND deleted_at IS NULL`, id).Scan(&live); err != nil {
		return err
	}
	if live > 0 {
		return fmt.Errorf("%w: %d", ErrDomainInUse, live)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM dns_zones WHERE id = $1::uuid`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DomainLiveSites returns how many live websites use a domain. It is used for
// the message shown before a delete.
func (s *Store) DomainLiveSites(ctx context.Context, id string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM sites WHERE zone_id = $1::uuid AND deleted_at IS NULL`, id).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, err
}
