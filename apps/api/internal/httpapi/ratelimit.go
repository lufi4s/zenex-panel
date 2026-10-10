package httpapi

import (
	"net/http"
	"sync"
	"time"
)

// fixedWindowLimiter allows at most limit events per key within each window.
// It is in-process; a multi-node deployment would move this to Redis.
type fixedWindowLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	counts map[string]windowCount
	now    func() time.Time
}

type windowCount struct {
	start time.Time
	n     int
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{limit: limit, window: window, counts: map[string]windowCount{}, now: time.Now}
}

// Allow records one event for key and reports whether it is within the limit.
func (l *fixedWindowLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	c := l.counts[key]
	if now.Sub(c.start) >= l.window {
		c = windowCount{start: now}
	}
	if c.n >= l.limit {
		l.counts[key] = c
		return false
	}
	c.n++
	l.counts[key] = c

	// Keep the map bounded: drop expired windows when it grows large.
	if len(l.counts) > 10000 {
		for k, v := range l.counts {
			if now.Sub(v.start) >= l.window {
				delete(l.counts, k)
			}
		}
	}
	return true
}

// limitActions runs next for a signed-in user, unless that user has used up the
// expensive-action allowance. It must sit inside requireSession.
func (d Deps) limitActions(next http.HandlerFunc) http.Handler {
	return d.requireSession(func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok || !d.ActionLimiter.Allow(user.ID) {
			writeError(w, requestIDFrom(r), ErrRateLimited)
			return
		}
		next(w, r)
	})
}
