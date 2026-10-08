package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// MonitorStore reads history recorded by the monitor and the audit log.
type MonitorStore interface {
	MetricSeries(ctx context.Context, since time.Time, bucket time.Duration) ([]store.MetricPoint, error)
	SiteHealthSince(ctx context.Context, ownerID string, since time.Time) ([]store.SiteHealth, error)
	ListActivity(ctx context.Context, actorID, action, result string, beforeID int64, limit int) ([]store.ActivityRow, error)
	JobLogs(ctx context.Context, jobID string) ([]store.JobLogLine, error)
}

// timeRanges maps the range selector to a window and a chart resolution.
var timeRanges = map[string]struct {
	window time.Duration
	bucket time.Duration
}{
	"1h":  {time.Hour, time.Minute},
	"6h":  {6 * time.Hour, 5 * time.Minute},
	"24h": {24 * time.Hour, 15 * time.Minute},
	"7d":  {7 * 24 * time.Hour, time.Hour},
}

func rangeFrom(r *http.Request) (window, bucket time.Duration, ok bool) {
	name := r.URL.Query().Get("range")
	if name == "" {
		name = "1h"
	}
	tr, found := timeRanges[name]
	return tr.window, tr.bucket, found
}

// handleMetricSeries returns host metrics history for the charts.
func (d Deps) handleMetricSeries(w http.ResponseWriter, r *http.Request) {
	window, bucket, ok := rangeFrom(r)
	if !ok {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	points, err := d.Monitor.MetricSeries(r.Context(), time.Now().Add(-window), bucket)
	if err != nil {
		d.internal(w, r, "monitoring.metrics", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"points": points, "bucket_seconds": int(bucket.Seconds())})
}

// handleSiteHealth summarises uptime checks for the sites the caller can see.
func (d Deps) handleSiteHealth(w http.ResponseWriter, r *http.Request) {
	window, _, ok := rangeFrom(r)
	if !ok {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	user, _ := currentUser(r)
	owner := user.ID
	if isAdmin(user) {
		owner = ""
	}
	list, err := d.Monitor.SiteHealthSince(r.Context(), owner, time.Now().Add(-window))
	if err != nil {
		d.internal(w, r, "monitoring.sites", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (d Deps) handleServices(w http.ResponseWriter, r *http.Request) {
	list, err := d.Manage.Services(r.Context())
	if err != nil {
		d.manageError(w, r, "monitoring.services", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type activityResponse struct {
	Items      []store.ActivityRow `json:"items"`
	NextBefore int64               `json:"next_before,omitempty"`
}

// handleActivity lists audit entries. Customers see only their own actions;
// administrators see everything.
func (d Deps) handleActivity(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r)
	q := r.URL.Query()

	limit := 50
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = min(v, 200)
	}
	var before int64
	if v := q.Get("before"); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil || parsed < 0 {
			writeError(w, requestIDFrom(r), ErrInvalidRequest)
			return
		}
		before = parsed
	}
	action := strings.TrimSpace(q.Get("action"))
	if len(action) > 64 || strings.ContainsAny(action, "%_\\") {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}
	result := q.Get("result")
	if result != "" && result != "success" && result != "failure" && result != "denied" {
		writeError(w, requestIDFrom(r), ErrInvalidRequest)
		return
	}

	actor := user.ID
	if isAdmin(user) {
		actor = ""
	}
	items, err := d.Monitor.ListActivity(r.Context(), actor, action, result, before, limit)
	if err != nil {
		d.internal(w, r, "activity.list", err)
		return
	}
	resp := activityResponse{Items: items}
	if len(items) == limit {
		resp.NextBefore = items[len(items)-1].ID
	}
	writeJSON(w, http.StatusOK, resp)
}

func (d Deps) handleJobLogs(w http.ResponseWriter, r *http.Request) {
	job, _, ok := d.loadJobForUser(w, r)
	if !ok {
		return
	}
	lines, err := d.Monitor.JobLogs(r.Context(), job.ID)
	if err != nil {
		d.internal(w, r, "jobs.logs", err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}
