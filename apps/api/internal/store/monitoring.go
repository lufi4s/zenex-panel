package store

import (
	"context"
	"time"
)

// MetricSample is one reading of host metrics.
type MetricSample struct {
	Time           time.Time
	CPUCount       int
	Load1m         float64
	MemUsedBytes   int64
	MemTotalBytes  int64
	DiskUsedBytes  int64
	DiskTotalBytes int64
}

// MetricPoint is a bucketed average used for charts.
type MetricPoint struct {
	Time     time.Time `json:"t"`
	Load     float64   `json:"load"`
	CPUCount int       `json:"cpus"`
	MemPct   float64   `json:"mem_pct"`
	DiskPct  float64   `json:"disk_pct"`
}

// SiteCheck is the result of one uptime probe.
type SiteCheck struct {
	SiteID     string
	OK         bool
	StatusCode int
	LatencyMS  int
	Error      string
}

// SiteHealth summarises a site's recent checks.
type SiteHealth struct {
	SiteID       string     `json:"site_id"`
	Checks       int        `json:"checks"`
	OKChecks     int        `json:"ok_checks"`
	AvgLatencyMS int        `json:"avg_latency_ms"`
	LastCheck    *time.Time `json:"last_check,omitempty"`
	LastOK       bool       `json:"last_ok"`
	LastStatus   int        `json:"last_status"`
}

// ActivityRow is one audit entry shown in the activity log.
type ActivityRow struct {
	ID         int64     `json:"id"`
	Time       time.Time `json:"time"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type,omitempty"`
	TargetID   string    `json:"target_id,omitempty"`
	Result     string    `json:"result"`
	ErrorCode  string    `json:"error_code,omitempty"`
	IP         string    `json:"ip,omitempty"`
	ActorRole  string    `json:"actor_role,omitempty"`
}

// JobLogLine is one line of a job's log.
type JobLogLine struct {
	ID      int64     `json:"id"`
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// InsertMetricSample stores one host reading.
func (s *Store) InsertMetricSample(ctx context.Context, m MetricSample) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO metric_samples (ts, cpu_count, load_1m, mem_used_bytes, mem_total_bytes, disk_used_bytes, disk_total_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		m.Time, m.CPUCount, m.Load1m, m.MemUsedBytes, m.MemTotalBytes, m.DiskUsedBytes, m.DiskTotalBytes)
	return err
}

// MetricSeries returns averages per bucket since the given time, oldest first.
func (s *Store) MetricSeries(ctx context.Context, since time.Time, bucket time.Duration) ([]MetricPoint, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT date_bin($1::interval, ts, 'epoch'::timestamptz) AS b,
		       avg(load_1m), max(cpu_count),
		       avg(mem_used_bytes::float8 / NULLIF(mem_total_bytes, 0)) * 100,
		       avg(disk_used_bytes::float8 / NULLIF(disk_total_bytes, 0)) * 100
		FROM metric_samples
		WHERE ts >= $2
		GROUP BY b
		ORDER BY b`, intervalString(bucket), since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MetricPoint{}
	for rows.Next() {
		var p MetricPoint
		var mem, disk *float64
		if err := rows.Scan(&p.Time, &p.Load, &p.CPUCount, &mem, &disk); err != nil {
			return nil, err
		}
		if mem != nil {
			p.MemPct = *mem
		}
		if disk != nil {
			p.DiskPct = *disk
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PruneMonitoring deletes history older than keep.
func (s *Store) PruneMonitoring(ctx context.Context, keep time.Duration) error {
	cutoff := time.Now().Add(-keep)
	if _, err := s.pool.Exec(ctx, `DELETE FROM metric_samples WHERE ts < $1`, cutoff); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM site_checks WHERE ts < $1`, cutoff)
	return err
}

// InsertSiteCheck records one uptime probe.
func (s *Store) InsertSiteCheck(ctx context.Context, c SiteCheck) error {
	var status, latency any
	if c.StatusCode > 0 {
		status = c.StatusCode
	}
	if c.LatencyMS >= 0 {
		latency = c.LatencyMS
	}
	var errText any
	if c.Error != "" {
		errText = truncate(c.Error, 300)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO site_checks (site_id, ok, status_code, latency_ms, error)
		VALUES ($1::uuid, $2, $3, $4, $5)`, c.SiteID, c.OK, status, latency, errText)
	return err
}

// LiveSite is the minimum the monitor needs to probe a website.
type LiveSite struct {
	ID      string
	Domain  string
	OwnerID string
}

// ReadySites returns every live website for uptime probes.
func (s *Store) ReadySites(ctx context.Context) ([]LiveSite, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, primary_domain::text, owner_user_id::text FROM sites
		WHERE state = 'ready' AND deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LiveSite
	for rows.Next() {
		var ls LiveSite
		if err := rows.Scan(&ls.ID, &ls.Domain, &ls.OwnerID); err != nil {
			return nil, err
		}
		out = append(out, ls)
	}
	return out, rows.Err()
}

// SiteHealthSince summarises checks per site since a time. An empty ownerID
// returns every site (administrators).
func (s *Store) SiteHealthSince(ctx context.Context, ownerID string, since time.Time) ([]SiteHealth, error) {
	rows, err := s.pool.Query(ctx, `
		WITH scoped AS (
			SELECT c.* FROM site_checks c
			JOIN sites st ON st.id = c.site_id
			WHERE c.ts >= $2 AND ($1 = '' OR st.owner_user_id = NULLIF($1, '')::uuid)
		)
		SELECT site_id::text,
		       count(*),
		       count(*) FILTER (WHERE ok),
		       COALESCE(avg(latency_ms) FILTER (WHERE ok), 0)::int,
		       max(ts),
		       COALESCE((array_agg(ok ORDER BY ts DESC))[1], false),
		       COALESCE((array_agg(status_code ORDER BY ts DESC))[1], 0)
		FROM scoped GROUP BY site_id`, ownerID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SiteHealth{}
	for rows.Next() {
		var h SiteHealth
		var last *time.Time
		if err := rows.Scan(&h.SiteID, &h.Checks, &h.OKChecks, &h.AvgLatencyMS, &last, &h.LastOK, &h.LastStatus); err != nil {
			return nil, err
		}
		h.LastCheck = last
		out = append(out, h)
	}
	return out, rows.Err()
}

// ListActivity returns audit entries newest first. actorID limits the list to one
// user (customers); an empty value returns everything (administrators). beforeID
// pages backwards. Action and result are optional filters.
func (s *Store) ListActivity(ctx context.Context, actorID, action, result string, beforeID int64, limit int) ([]ActivityRow, error) {
	var before any
	if beforeID > 0 {
		before = beforeID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, occurred_at, action, COALESCE(target_type, ''), COALESCE(target_id, ''),
		       result, COALESCE(error_code, ''), COALESCE(host(ip_address), ''), COALESCE(actor_role, '')
		FROM audit_logs
		WHERE ($1 = '' OR actor_user_id = NULLIF($1, '')::uuid)
		  AND ($2 = '' OR action ILIKE '%' || $2 || '%')
		  AND ($3 = '' OR result = $3)
		  AND ($4::bigint IS NULL OR id < $4::bigint)
		ORDER BY id DESC
		LIMIT $5`, actorID, action, result, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActivityRow{}
	for rows.Next() {
		var a ActivityRow
		if err := rows.Scan(&a.ID, &a.Time, &a.Action, &a.TargetType, &a.TargetID, &a.Result, &a.ErrorCode, &a.IP, &a.ActorRole); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// JobLogs returns a job's log lines in order.
func (s *Store) JobLogs(ctx context.Context, jobID string) ([]JobLogLine, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, created_at, level, message FROM job_logs
		WHERE job_id = $1::uuid ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JobLogLine{}
	for rows.Next() {
		var l JobLogLine
		if err := rows.Scan(&l.ID, &l.Time, &l.Level, &l.Message); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
