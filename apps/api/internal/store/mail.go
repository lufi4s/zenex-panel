package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ZoneInfo is the part of a connected domain the mail code needs.
type ZoneInfo struct {
	ID      string
	OwnerID string
	Apex    string
}

// MailDomain says whether mail is turned on for a connected domain.
type MailDomain struct {
	ZoneID     string
	Enabled    bool
	DKIMPublic string // the p= value of the domain's DKIM key, empty until it exists
}

// Mailbox is one address on a connected domain. PasswordHash is never sent to clients.
type Mailbox struct {
	ID           string    `json:"id"`
	ZoneID       string    `json:"zone_id"`
	LocalPart    string    `json:"local_part"`
	PasswordHash string    `json:"-"`
	QuotaMB      int       `json:"quota_mb"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ZoneOwner returns a connected domain and its owner, for any owner (the caller checks access).
func (s *Store) ZoneOwner(ctx context.Context, zoneID string) (ZoneInfo, error) {
	var z ZoneInfo
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, owner_user_id::text, apex_domain::text FROM dns_zones WHERE id = $1::uuid`, zoneID,
	).Scan(&z.ID, &z.OwnerID, &z.Apex)
	if errors.Is(err, pgx.ErrNoRows) {
		return ZoneInfo{}, ErrNotFound
	}
	return z, err
}

// GetMailDomain returns the mail setting of a connected domain. A domain without a row has mail off.
func (s *Store) GetMailDomain(ctx context.Context, zoneID string) (MailDomain, error) {
	m := MailDomain{ZoneID: zoneID}
	err := s.pool.QueryRow(ctx,
		`SELECT enabled, dkim_public FROM mail_domains WHERE zone_id = $1::uuid`, zoneID).Scan(&m.Enabled, &m.DKIMPublic)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, nil
	}
	return m, err
}

// SetMailDomain turns mail on or off for a connected domain.
func (s *Store) SetMailDomain(ctx context.Context, zoneID string, enabled bool) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mail_domains (zone_id, enabled) VALUES ($1::uuid, $2)
		ON CONFLICT (zone_id) DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = now()`, zoneID, enabled)
	return err
}

// SetMailDKIM stores the public DKIM value of a domain.
func (s *Store) SetMailDKIM(ctx context.Context, zoneID, value string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO mail_domains (zone_id, enabled, dkim_public) VALUES ($1::uuid, false, $2)
		ON CONFLICT (zone_id) DO UPDATE SET dkim_public = EXCLUDED.dkim_public, updated_at = now()`, zoneID, value)
	return err
}

// mailHostKey is the system setting that holds the name of the mail server.
const mailHostKey = "mail_host"

// GetMailHost returns the saved mail server name, or "" when none is saved.
func (s *Store) GetMailHost(ctx context.Context) (string, error) {
	raw, err := s.GetSetting(ctx, mailHostKey)
	if errors.Is(err, ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var host string
	if err := json.Unmarshal(raw, &host); err != nil {
		return "", err
	}
	return host, nil
}

// SetMailHost saves the mail server name and records who set it.
func (s *Store) SetMailHost(ctx context.Context, host, userID string) error {
	return s.PutSetting(ctx, mailHostKey, host, userID)
}

// ListMailboxes returns the mailboxes of a connected domain, sorted by address.
func (s *Store) ListMailboxes(ctx context.Context, zoneID string) ([]Mailbox, error) {
	rows, err := s.pool.Query(ctx, mailboxColumns+` WHERE zone_id = $1::uuid ORDER BY local_part`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Mailbox{}
	for rows.Next() {
		mb, err := scanMailbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, mb)
	}
	return out, rows.Err()
}

// CreateMailbox adds a mailbox. An address already taken on the domain gives ErrConflict.
func (s *Store) CreateMailbox(ctx context.Context, mb Mailbox) (Mailbox, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO mailboxes (zone_id, local_part, password_hash, quota_mb, active)
		VALUES ($1::uuid, $2, $3, $4, $5)
		RETURNING id::text, zone_id::text, local_part::text, password_hash, quota_mb, active, created_at, updated_at`,
		mb.ZoneID, mb.LocalPart, mb.PasswordHash, mb.QuotaMB, mb.Active)
	out, err := scanMailbox(row)
	if isUniqueViolation(err) {
		return Mailbox{}, ErrConflict
	}
	return out, err
}

// GetMailbox returns one mailbox by ID.
func (s *Store) GetMailbox(ctx context.Context, id string) (Mailbox, error) {
	mb, err := scanMailbox(s.pool.QueryRow(ctx, mailboxColumns+` WHERE id = $1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Mailbox{}, ErrNotFound
	}
	return mb, err
}

// UpdateMailbox saves the password hash, quota and active flag of a mailbox.
func (s *Store) UpdateMailbox(ctx context.Context, mb Mailbox) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE mailboxes SET password_hash = $2, quota_mb = $3, active = $4, updated_at = now()
		WHERE id = $1::uuid`, mb.ID, mb.PasswordHash, mb.QuotaMB, mb.Active)
	return err
}

// DeleteMailbox removes a mailbox.
func (s *Store) DeleteMailbox(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM mailboxes WHERE id = $1::uuid`, id)
	return err
}

const mailboxColumns = `SELECT id::text, zone_id::text, local_part::text, password_hash, quota_mb, active, created_at, updated_at FROM mailboxes`

func scanMailbox(row pgx.Row) (Mailbox, error) {
	var mb Mailbox
	err := row.Scan(&mb.ID, &mb.ZoneID, &mb.LocalPart, &mb.PasswordHash, &mb.QuotaMB, &mb.Active, &mb.CreatedAt, &mb.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Mailbox{}, ErrNotFound
	}
	return mb, err
}
