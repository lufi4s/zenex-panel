package store

import (
	"context"
	"time"
)

// ActivityStep is one step of a job, in order.
type ActivityStep struct {
	Name   string
	Status string
}

// SiteActivity is the latest job of a website that is still running, or that finished a moment ago.
type SiteActivity struct {
	SiteID string
	JobID  string
	Type   string
	Status string
	Steps  []ActivityStep
}

// ListSiteActivity returns, for each website the owner can see ("" means every website), its
// latest job of the given types when that job is queued or running, or finished within the last
// `recent` period. The result lets the website list show "Backing up 45%" or "Backup complete".
func (s *Store) ListSiteActivity(ctx context.Context, ownerID string, types []string, recent time.Duration) ([]SiteActivity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (j.site_id) j.site_id::text, j.id::text, j.type, j.status
		FROM jobs j JOIN sites s ON s.id = j.site_id
		WHERE s.deleted_at IS NULL
		  AND ($1 = '' OR s.owner_user_id = NULLIF($1, '')::uuid)
		  AND j.type = ANY($2::text[])
		  AND (j.status IN ('queued', 'running')
		       OR COALESCE(j.finished_at, j.updated_at) > now() - make_interval(secs => $3::double precision))
		ORDER BY j.site_id, j.created_at DESC`,
		ownerID, types, recent.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SiteActivity
	ids := []string{}
	for rows.Next() {
		var a SiteActivity
		if err := rows.Scan(&a.SiteID, &a.JobID, &a.Type, &a.Status); err != nil {
			return nil, err
		}
		out = append(out, a)
		ids = append(ids, a.JobID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return []SiteActivity{}, nil
	}

	stepRows, err := s.pool.Query(ctx, `
		SELECT job_id::text, name, status FROM job_steps
		WHERE job_id::text = ANY($1::text[]) ORDER BY job_id, seq`, ids)
	if err != nil {
		return nil, err
	}
	defer stepRows.Close()
	steps := map[string][]ActivityStep{}
	for stepRows.Next() {
		var jobID string
		var st ActivityStep
		if err := stepRows.Scan(&jobID, &st.Name, &st.Status); err != nil {
			return nil, err
		}
		steps[jobID] = append(steps[jobID], st)
	}
	if err := stepRows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Steps = steps[out[i].JobID]
	}
	return out, nil
}

// FailInterruptedJobs marks backup, restore and migration jobs that were running when the panel
// stopped as failed. They run in the panel's own process, so after a restart nothing is running
// them any more, and without this they would show as "running" for ever and block the website.
func (s *Store) FailInterruptedJobs(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE jobs
		SET status = 'failed', error_message = 'The panel restarted while this was running.',
		    finished_at = now(), updated_at = now()
		WHERE status IN ('queued', 'running')
		  AND type IN ('site.backup', 'site.restore', 'site.migrate')`)
	return err
}
