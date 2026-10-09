package schedule

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestDueOnlyInTheScheduledHourAndOncePerDay(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 10, 9, h, m, 0, 0, time.Local) }

	if !Due(at(4, 0), 4, "") {
		t.Fatal("not due at the scheduled hour")
	}
	if !Due(at(4, 59), 4, "") {
		t.Fatal("not due at any minute of the scheduled hour")
	}
	if Due(at(3, 59), 4, "") {
		t.Fatal("due before the scheduled hour")
	}
	if Due(at(4, 0), 4, "2026-10-09") {
		t.Fatal("due twice on the same day")
	}
	if !Due(at(4, 0), 4, "2026-10-08") {
		t.Fatal("not due on a new day")
	}
	if Due(at(4, 0), -1, "") || Due(at(4, 0), 24, "") {
		t.Fatal("skipped or invalid hour was due")
	}
}

func TestRunnerStartsTaskOnceAndSkipsDisabledOnes(t *testing.T) {
	now := time.Date(2026, 10, 9, 4, 30, 0, 0, time.Local)
	r := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.Now = func() time.Time { return now }

	var mu sync.Mutex
	runs := map[string]int{}
	done := make(chan string, 4)
	record := func(name string) func(context.Context) {
		return func(context.Context) {
			mu.Lock()
			runs[name]++
			mu.Unlock()
			done <- name
		}
	}
	r.Add(Task{Name: "updates", Hour: func(context.Context) int { return 4 }, Run: record("updates")})
	r.Add(Task{Name: "backups", Hour: func(context.Context) int { return -1 }, Run: record("backups")})

	ctx := context.Background()
	r.tick(ctx)
	r.tick(ctx) // same day: must not start again

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("due task never started")
	}
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if runs["updates"] != 1 || runs["backups"] != 0 {
		t.Fatalf("runs = %v", runs)
	}
}
