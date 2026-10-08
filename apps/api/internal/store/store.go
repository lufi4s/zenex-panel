// Package store is the PostgreSQL persistence layer for the control plane.
package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	pool *pgxpool.Pool
}

// Open connects to PostgreSQL and verifies the connection.
func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

// Migrate applies every embedded *.sql file not yet recorded in schema_migrations.
// Each file runs in its own transaction, so a failed migration leaves no partial state.
func (s *Store) Migrate(ctx context.Context, files fs.FS) error {
	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, name,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check %s: %w", name, err)
		}
		if applied {
			continue
		}
		sqlBytes, err := fs.ReadFile(files, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := s.applyFile(ctx, name, string(sqlBytes)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) applyFile(ctx context.Context, name, sql string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("apply %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
		return fmt.Errorf("record %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", name, err)
	}
	return nil
}

// User is an account as needed for login and authorization.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Status       string
	FailedLogins int
	LockedUntil  *time.Time
	Roles        []string
}

const userColumns = `
	u.id::text,
	u.email::text,
	u.password_hash,
	u.status,
	u.failed_logins,
	u.locked_until,
	COALESCE((SELECT array_agg(r.name ORDER BY r.name)
	          FROM user_roles ur JOIN roles r ON r.id = ur.role_id
	          WHERE ur.user_id = u.id), '{}')`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Status, &u.FailedLogins, &u.LockedUntil, &u.Roles); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users u WHERE u.email = $1`, email))
}

// CreateUser inserts an active user with a single role.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash, role string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id::text`,
		email, passwordHash,
	).Scan(&id); err != nil {
		return "", err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1::uuid, id FROM roles WHERE name = $2`, id, role)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", fmt.Errorf("role %q does not exist", role)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// UpsertAdmin creates the administrator account, or resets its password if it
// already exists. It is safe to run repeatedly, so the installer never gets stuck
// on "user already exists".
func (s *Store) UpsertAdmin(ctx context.Context, email, passwordHash string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash) VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE
		SET password_hash = EXCLUDED.password_hash,
		    status = 'active', failed_logins = 0, locked_until = NULL, updated_at = now()
		RETURNING id::text`, email, passwordHash).Scan(&id); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1::uuid, id FROM roles WHERE name = 'administrator'
		ON CONFLICT DO NOTHING`, id); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// RecordFailedLogin increments the counter and locks the account after maxFailures.
func (s *Store) RecordFailedLogin(ctx context.Context, userID string, maxFailures int, lockFor time.Duration) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users
		SET failed_logins = failed_logins + 1,
		    locked_until  = CASE WHEN failed_logins + 1 >= $2
		                         THEN now() + $3::interval
		                         ELSE locked_until END,
		    updated_at    = now()
		WHERE id = $1::uuid`,
		userID, maxFailures, intervalString(lockFor))
	return err
}

// RecordSuccessfulLogin clears failures and stamps last_login_at.
func (s *Store) RecordSuccessfulLogin(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE users SET failed_logins = 0, locked_until = NULL, last_login_at = now(), updated_at = now()
		WHERE id = $1::uuid`, userID)
	return err
}

type SessionInput struct {
	UserID    string
	TokenHash []byte
	IP        string
	UserAgent string
	ExpiresAt time.Time
}

func (s *Store) CreateSession(ctx context.Context, in SessionInput) error {
	var ip any
	if in.IP != "" {
		ip = in.IP
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, ip_address, user_agent, expires_at)
		VALUES ($1::uuid, $2, $3::inet, $4, $5)`,
		in.UserID, in.TokenHash, ip, truncate(in.UserAgent, 256), in.ExpiresAt)
	return err
}

// SessionUser returns the account behind an active, unexpired, unrevoked session.
func (s *Store) SessionUser(ctx context.Context, tokenHash []byte) (*User, error) {
	return scanUser(s.pool.QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM sessions se
		JOIN users u ON u.id = se.user_id
		WHERE se.token_hash = $1
		  AND se.revoked_at IS NULL
		  AND se.expires_at > now()
		  AND u.status = 'active'`, tokenHash))
}

func (s *Store) RevokeSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	return err
}

type AuditEntry struct {
	ActorUserID string // empty for anonymous actions
	ActorRole   string
	Action      string
	TargetType  string
	TargetID    string
	IP          string
	Result      string // success, failure, denied
	ErrorCode   string
}

// Audit appends one row to audit_logs. Metadata is intentionally not accepted here,
// so callers cannot accidentally write secrets into it.
func (s *Store) Audit(ctx context.Context, e AuditEntry) error {
	var actor, ip any
	if e.ActorUserID != "" {
		actor = e.ActorUserID
	}
	if e.IP != "" {
		ip = e.IP
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, actor_role, action, target_type, target_id, ip_address, result, error_code)
		VALUES ($1::uuid, NULLIF($2, ''), $3, NULLIF($4, ''), NULLIF($5, ''), $6::inet, $7, NULLIF($8, ''))`,
		actor, e.ActorRole, e.Action, e.TargetType, e.TargetID, ip, e.Result, e.ErrorCode)
	return err
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func intervalString(d time.Duration) string {
	return fmt.Sprintf("%d seconds", int(d.Seconds()))
}

// truncate limits s to n runes so multi-byte characters are never split.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
