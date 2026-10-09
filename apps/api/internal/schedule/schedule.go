// Package schedule runs daily tasks at a wall-clock hour in server time.
package schedule

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Task runs once per day, during the hour Hour returns. A negative hour skips the day.
type Task struct {
	Name string
	Hour func(ctx context.Context) int
	Run  func(ctx context.Context)
}

// Runner checks the clock every minute and starts tasks that are due.
type Runner struct {
	Log   *slog.Logger
	Now   func() time.Time
	Every time.Duration

	mu    sync.Mutex
	tasks []Task
	last  map[string]string // task name -> date (YYYY-MM-DD) it last started
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
func (r *Runner) tick(ctx context.Context) {
	now := r.Now()
	day := now.Format("2006-01-02")
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tasks {
		if !Due(now, t.Hour(ctx), r.last[t.Name]) {
			continue
		}
		r.last[t.Name] = day
		r.Log.Info("starting daily task", "task", t.Name)
		go t.Run(ctx)
	}
}

// Due reports whether a task scheduled for hour should start at now, given the day
// (YYYY-MM-DD) it last started. Each task starts at most once per day.
func Due(now time.Time, hour int, lastDay string) bool {
	if hour < 0 || hour > 23 {
		return false
	}
	return now.Hour() == hour && now.Format("2006-01-02") != lastDay
}
