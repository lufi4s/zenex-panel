// Package schedule runs recurring tasks at wall-clock times in server time.
package schedule

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Frequencies a task can run at.
const (
	FrequencyHourly = "hourly"
	FrequencyDaily  = "daily"
	FrequencyWeekly = "weekly"
)

// Plan says when a task runs. Hourly runs start at minute 0 of every hour and ignore
// Hour and Weekday. Daily runs start during Hour every day. Weekly runs start during
// Hour on Weekday (0-6, Sunday = 0). An empty Frequency means daily.
type Plan struct {
	Frequency string
	Hour      int
	Weekday   int
}

// Task runs at the times Plan describes. The plan is read on every check, so a
// change in settings applies on the next minute.
type Task struct {
	Name string
	Plan func(ctx context.Context) Plan
	Run  func(ctx context.Context)
}

// Runner checks the clock every minute and starts tasks that are due.
type Runner struct {
	Log   *slog.Logger
	Now   func() time.Time
	Every time.Duration

	mu    sync.Mutex
	tasks []Task
	last  map[string]string // task name -> run key of its last start
}

// New returns a runner that checks once a minute.
func New(log *slog.Logger) *Runner {
	return &Runner{Log: log, Now: time.Now, Every: time.Minute, last: map[string]string{}}
}

// Add registers a task.
func (r *Runner) Add(t Task) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks = append(r.tasks, t)
}

// Run blocks until ctx is cancelled.
func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.Every)
	defer ticker.Stop()
	r.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tick(ctx)
		}
	}
}

// tick starts every task that is due. Each task runs in its own goroutine, so a
// long backup never delays the others.
//
// The last run key is kept in memory only. A restart in the same period as a run
// (for example the same hour for an hourly task) can therefore repeat that run once.
func (r *Runner) tick(ctx context.Context) {
	now := r.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		key, due := RunKey(now, t.Plan(ctx))
		if !due || key == r.last[t.Name] {
			continue
		}
		r.last[t.Name] = key
		r.Log.Info("starting scheduled task", "task", t.Name, "run", key)
		go t.Run(ctx)
	}
}

// RunKey reports whether a task with plan p is due at now, and the key that
// identifies this run period (for example "2026-10-11T03" for hourly, "2026-10-11"
// for daily and "2026-W41" for weekly). Each period has at most one run; compare the
// key with the last one started to avoid repeats.
func RunKey(now time.Time, p Plan) (string, bool) {
	switch p.Frequency {
	case "", FrequencyDaily:
		if !validHour(p.Hour) || now.Hour() != p.Hour {
			return "", false
		}
		return now.Format("2006-01-02"), true
	case FrequencyHourly:
		if now.Minute() != 0 {
			return "", false
		}
		return now.Format("2006-01-02T15"), true
	case FrequencyWeekly:
		if !validHour(p.Hour) || p.Weekday < 0 || p.Weekday > 6 {
			return "", false
		}
		if now.Weekday() != time.Weekday(p.Weekday) || now.Hour() != p.Hour {
			return "", false
		}
		year, week := now.ISOWeek()
		return fmt.Sprintf("%04d-W%02d", year, week), true
	}
	return "", false
}

// Due reports whether a task with plan p should start at now, given the run key of
// its last start (see RunKey). Each run period starts at most once.
func Due(now time.Time, p Plan, lastKey string) bool {
	key, ok := RunKey(now, p)
	return ok && key != lastKey
}

func validHour(h int) bool {
	return h >= 0 && h <= 23
}
