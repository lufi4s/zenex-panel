package httpapi

import (
	"net/http"
	"testing"
	"time"
)

func TestFixedWindowLimiter(t *testing.T) {
	clock := time.Unix(1000, 0)
	l := newFixedWindowLimiter(2, time.Minute)
	l.now = func() time.Time { return clock }

	if !l.Allow("ip") || !l.Allow("ip") {
		t.Fatal("first two events should pass")
	}
	if l.Allow("ip") {
		t.Fatal("third event in window should be blocked")
	}
	if !l.Allow("other") {
		t.Fatal("limit must be per key")
	}
	clock = clock.Add(time.Minute)
	if !l.Allow("ip") {
		t.Fatal("event after window reset should pass")
	}
}

// Expensive actions stop after the per-user allowance, so one account cannot start
// SSH logins, scans or backups in a loop. The allowance is counted per user.
func TestExpensiveActionsAreRateLimitedPerUser(t *testing.T) {
	e := newSettingsEnv(t)
	admin := e.login(t, "admin@example.com")
	const path = "/api/v1/settings/backups/run-now"
	for i := 0; i < 20; i++ {
		if rec := e.call(t, http.MethodPost, path, "", admin, true); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d limited too early", i+1)
		}
	}
	if rec := e.call(t, http.MethodPost, path, "", admin, true); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("request 21 = %d, want 429", rec.Code)
	}
}
