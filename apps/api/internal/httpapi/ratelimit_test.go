package httpapi

import (
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
