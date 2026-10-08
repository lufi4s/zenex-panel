package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrConflict means a unique value (domain, slug, idempotency key) is already taken.
var ErrConflict = errors.New("already exists")

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Domain is a customer's connected apex domain (e.g. example.com).
type Domain struct {
	ID        string     `json:"id"`
	Apex      string     `json:"apex"`
	Verified  bool       `json:"verified"`
	Message   string     `json:"message"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Site is a WordPress site hosted on a node.
type Site struct {
	ID         string    `json:"id"`
	OwnerID    string    `json:"owner_user_id"`
	NodeID     string    `json:"node_id"`
	Slug       string    `json:"slug"`
	Domain     string    `json:"domain"`
	LinuxUser  string    `json:"-"`
	DBName     string    `json:"-"`
	DBUser     string    `json:"-"`
	PHPVersion string    `json:"php_version"`
	State      string    `json:"state"`
	Health     string    `json:"health"`
	CreatedAt  time.Time `json:"created_at"`
}

// Job is a long-running operation tracked in the jobs table.
type Job struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	ActorID  string `json:"-"`
	SiteID   string `json:"site_id,omitempty"`
	Attempts int    `json:"attempts"`
	Error    string `json:"error,omitempty"`
}

// JobStep is one ordered step of a job.
type JobStep struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Attempts int    `json:"attempts"`
	Error    string `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

// EnsureLocalNode returns the node for this server, creating it on first use.
// The node is owned by the first administrator.
func (s *Store) EnsureLocalNode(ctx context.Context, name, publicIP string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text FROM vps_nodes WHERE name = $1 AND status <> 'decommissioned'`, name,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO vps_nodes (owner_user_id, name, public_ip, status, last_seen_at)
		SELECT ur.user_id, $1, $2::inet, 'online', now()
		FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		WHERE r.name = 'administrator'
		ORDER BY ur.user_id
		LIMIT 1
		RETURNING id::text`, name, publicIP).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("no administrator account exists; create one first")
	}
	return id, err
}

// ---------------------------------------------------------------------------
// Domains
// ---------------------------------------------------------------------------

func (s *Store) CreateDomain(ctx context.Context, ownerID, apex string) (Domain, error) {
	var d Domain
	err := s.pool.QueryRow(ctx, `
		INSERT INTO dns_zones (owner_user_id, apex_domain, mode)
		VALUES ($1::uuid, $2, 'external')
		RETURNING id::text, apex_domain::text, created_at`, ownerID, apex,
	).Scan(&d.ID, &d.Apex, &d.CreatedAt)
	if isUniqueViolation(err) {
		return Domain{}, ErrConflict
	}
	return d, err
}

func (s *Store) ListDomains(ctx context.Context, ownerID string) ([]Domain, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+domainColumns+`
		FROM dns_zones WHERE owner_user_id = $1::uuid ORDER BY apex_domain`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DomainOwnedBy returns the zone ID when ownerID has connected apex.
func (s *Store) DomainOwnedBy(ctx context.Context, ownerID, apex string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text FROM dns_zones WHERE owner_user_id = $1::uuid AND apex_domain = $2`,
		ownerID, apex).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// ---------------------------------------------------------------------------
// Sites and provisioning jobs
// ---------------------------------------------------------------------------

// NewSite describes a site to create together with its provisioning job.
type NewSite struct {
	OwnerID        string
	NodeID         string
	ZoneID         string
	Slug           string
	Domain         string
	LinuxUser      string
	DBName         string
	DBUser         string
	PHPVersion     string
	IdempotencyKey string
	JobType        string
}

// CreateSiteWithJob inserts the site, its primary domain and a queued job in one
// transaction. With an idempotency key, a repeated request returns the original
// job and site instead of creating a second one. existed reports that case.
func (s *Store) CreateSiteWithJob(ctx context.Context, in NewSite) (siteID, jobID string, existed bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var key any
	if in.IdempotencyKey != "" {
		key = in.IdempotencyKey
		err = tx.QueryRow(ctx, `
			SELECT id::text, COALESCE(site_id::text, '') FROM jobs
			WHERE actor_user_id = $1::uuid AND idempotency_key = $2`,
			in.OwnerID, in.IdempotencyKey).Scan(&jobID, &siteID)
		if err == nil {
			return siteID, jobID, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", "", false, err
		}
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO sites (owner_user_id, node_id, zone_id, slug, primary_domain, linux_user,
		                   php_version, db_name, db_user, state)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, 'provisioning')
		RETURNING id::text`,
		in.OwnerID, in.NodeID, in.ZoneID, in.Slug, in.Domain, in.LinuxUser,
		in.PHPVersion, in.DBName, in.DBUser,
	).Scan(&siteID)
	if isUniqueViolation(err) {
		return "", "", false, ErrConflict
	}
	if err != nil {
		return "", "", false, err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO domains (site_id, fqdn, kind) VALUES ($1::uuid, $2, 'primary')`,
		siteID, in.Domain); err != nil {
		if isUniqueViolation(err) {
			return "", "", false, ErrConflict
		}
		return "", "", false, err
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO jobs (type, status, actor_user_id, node_id, site_id, idempotency_key)
		VALUES ($1, 'queued', $2::uuid, $3::uuid, $4::uuid, $5)
		RETURNING id::text`,
		in.JobType, in.OwnerID, in.NodeID, siteID, key,
	).Scan(&jobID)
	if err != nil {
		return "", "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", false, err
	}
	return siteID, jobID, false, nil
}

const siteColumns = `
	s.id::text, s.owner_user_id::text, s.node_id::text, s.slug, s.primary_domain::text,
	s.linux_user, s.db_name, s.db_user, s.php_version, s.state, s.health, s.created_at`

func scanSite(row pgx.Row) (Site, error) {
	var st Site
	err := row.Scan(&st.ID, &st.OwnerID, &st.NodeID, &st.Slug, &st.Domain,
		&st.LinuxUser, &st.DBName, &st.DBUser, &st.PHPVersion, &st.State, &st.Health, &st.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Site{}, ErrNotFound
	}
	return st, err
}

func (s *Store) GetSite(ctx context.Context, id string) (Site, error) {
	return scanSite(s.pool.QueryRow(ctx,
		`SELECT `+siteColumns+` FROM sites s WHERE s.id = $1::uuid AND s.deleted_at IS NULL`, id))
}

// ListSites returns the owner's sites, or every site when ownerID is empty (administrators).
func (s *Store) ListSites(ctx context.Context, ownerID string) ([]Site, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+siteColumns+` FROM sites s
		WHERE s.deleted_at IS NULL AND ($1 = '' OR s.owner_user_id = NULLIF($1, '')::uuid)
		ORDER BY s.created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Site{}
	for rows.Next() {
		st, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) SetSiteState(ctx context.Context, siteID, state string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sites SET state = $2, updated_at = now() WHERE id = $1::uuid`, siteID, state)
	return err
}

func (s *Store) SetSiteUID(ctx context.Context, siteID string, uid int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sites SET linux_uid = $2, updated_at = now() WHERE id = $1::uuid`, siteID, uid)
	return err
}

// SiteOwnerEmail returns the owner's login email for WordPress setup.
func (s *Store) SiteOwnerEmail(ctx context.Context, siteID string) (string, error) {
	var email string
	err := s.pool.QueryRow(ctx, `
		SELECT u.email::text FROM sites s JOIN users u ON u.id = s.owner_user_id
		WHERE s.id = $1::uuid`, siteID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return email, err
}

// ---------------------------------------------------------------------------
// Jobs
// ---------------------------------------------------------------------------

const jobColumns = `
	j.id::text, j.type, j.status, COALESCE(j.actor_user_id::text, ''),
	COALESCE(j.site_id::text, ''), j.attempts, COALESCE(j.error_message, '')`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.Type, &j.Status, &j.ActorID, &j.SiteID, &j.Attempts, &j.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return j, err
}

func (s *Store) GetJob(ctx context.Context, id string) (Job, error) {
	return scanJob(s.pool.QueryRow(ctx,
		`SELECT `+jobColumns+` FROM jobs j WHERE j.id = $1::uuid`, id))
}

// LatestJobForSite returns the most recent job for a site.
func (s *Store) LatestJobForSite(ctx context.Context, siteID string) (Job, error) {
	return scanJob(s.pool.QueryRow(ctx,
		`SELECT `+jobColumns+` FROM jobs j WHERE j.site_id = $1::uuid
		 ORDER BY j.created_at DESC LIMIT 1`, siteID))
}

// JobSteps returns the steps of a job in execution order.
func (s *Store) JobSteps(ctx context.Context, jobID string) ([]JobStep, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT name, status, attempts, COALESCE(error_message, '')
		FROM job_steps WHERE job_id = $1::uuid ORDER BY seq`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JobStep{}
	for rows.Next() {
		var st JobStep
		if err := rows.Scan(&st.Name, &st.Status, &st.Attempts, &st.Error); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// EnsureJobSteps creates any missing step rows, keeping existing progress.
func (s *Store) EnsureJobSteps(ctx context.Context, jobID string, names []string) error {
	for i, name := range names {
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO job_steps (job_id, seq, name) VALUES ($1::uuid, $2, $3)
			ON CONFLICT (job_id, name) DO NOTHING`, jobID, i, name); err != nil {
			return fmt.Errorf("create step %s: %w", name, err)
		}
	}
	return nil
}

// ClaimJob moves a queued or failed job to running. It returns false when another
// worker already holds it, so a job is never run twice at the same time.
func (s *Store) ClaimJob(ctx context.Context, jobID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET status = 'running', attempts = attempts + 1, started_at = now(),
		    finished_at = NULL, error_message = NULL, updated_at = now()
		WHERE id = $1::uuid AND status IN ('queued', 'failed')`, jobID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// RequeueFailedJob lets an operator retry a failed job. It returns false if the
// job is not in the failed state.
func (s *Store) RequeueFailedJob(ctx context.Context, jobID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE jobs SET status = 'queued', updated_at = now()
		WHERE id = $1::uuid AND status = 'failed'`, jobID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) SetStepStatus(ctx context.Context, jobID, name, status, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE job_steps
		SET status = $3::text,
		    attempts = attempts + CASE WHEN $3::text = 'running' THEN 1 ELSE 0 END,
		    error_message = NULLIF($4::text, ''),
		    started_at = CASE WHEN $3::text = 'running' THEN now() ELSE started_at END,
		    finished_at = CASE WHEN $3::text IN ('succeeded', 'failed') THEN now() END
		WHERE job_id = $1::uuid AND name = $2`, jobID, name, status, errMsg)
	return err
}

func (s *Store) FinishJob(ctx context.Context, jobID, status, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET status = $2::text, error_message = NULLIF($3::text, ''),
		    finished_at = CASE WHEN $2::text IN ('succeeded', 'failed', 'dead') THEN now() END,
		    updated_at = now()
		WHERE id = $1::uuid`, jobID, status, errMsg)
	return err
}

// UnfinishedProvisionJobs lists provisioning jobs that a restart interrupted.
func (s *Store) UnfinishedProvisionJobs(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text FROM jobs WHERE type = 'site.provision' AND status IN ('queued', 'running')`)
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

// ResetRunningJobs marks jobs left in "running" by a crash as queued.
func (s *Store) ResetRunningJobs(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs SET status = 'queued', updated_at = now()
		WHERE status = 'running' AND type = 'site.provision'`)
	return err
}

// AppendJobLog records a human-readable line for a job. Never pass secrets.
func (s *Store) AppendJobLog(ctx context.Context, jobID, level, msg string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO job_logs (job_id, level, message) VALUES ($1::uuid, $2, $3)`, jobID, level, msg)
	return err
}

const domainColumns = `id::text, apex_domain::text, verified_at IS NOT NULL,
	COALESCE(check_message, ''), last_checked_at, created_at`

func scanDomain(row pgx.Row) (Domain, error) {
	var d Domain
	err := row.Scan(&d.ID, &d.Apex, &d.Verified, &d.Message, &d.CheckedAt, &d.CreatedAt)
	return d, err
}

// GetDomainOwned returns one of the owner's domains.
func (s *Store) GetDomainOwned(ctx context.Context, ownerID, id string) (Domain, error) {
	d, err := scanDomain(s.pool.QueryRow(ctx, `
		SELECT `+domainColumns+` FROM dns_zones
		WHERE id = $1::uuid AND owner_user_id = $2::uuid`, id, ownerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, ErrNotFound
	}
	return d, err
}

// SetDomainCheck records the outcome of a DNS check.
func (s *Store) SetDomainCheck(ctx context.Context, id string, verified bool, message string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE dns_zones
		SET verified_at = CASE WHEN $2::boolean THEN COALESCE(verified_at, now()) ELSE NULL END,
		    check_message = $3, last_checked_at = now()
		WHERE id = $1::uuid`, id, verified, message)
	return err
}

// FindSiteByIdempotencyKey returns the site and job created with this key, so a
// repeated request is answered without creating anything again.
func (s *Store) FindSiteByIdempotencyKey(ctx context.Context, actorID, key string) (string, string, error) {
	var siteID, jobID string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(site_id::text, ''), id::text FROM jobs
		WHERE actor_user_id = $1::uuid AND idempotency_key = $2`, actorID, key).Scan(&siteID, &jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return siteID, jobID, err
}
