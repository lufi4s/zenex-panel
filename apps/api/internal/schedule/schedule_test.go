package schedule

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// 2026-10-11 is a Sunday; 2026-10-10 is a Saturday; 2026-10-12 is a Monday.
func at(day, h, m int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, time.Local) }

func TestWeekdayFixtures(t *testing.T) {
	if at(11, 0, 0).Weekday() != time.Sunday || at(10, 0, 0).Weekday() != time.Saturday || at(12, 0, 0).Weekday() != time.Monday {
		t.Fatal("test calendar is wrong: 11 Oct 2026 must be a Sunday")
	}
}

func TestDailyDueOnlyInTheScheduledHourAndOncePerDay(t *testing.T) {
	daily := Plan{Frequency: FrequencyDaily, Hour: 4}
	if !Due(at(9, 4, 0), daily, "") {
		t.Fatal("not due at the scheduled hour")
	}
	if !Due(at(9, 4, 59), daily, "") {
		t.Fatal("not due at any minute of the scheduled hour")
	}
	if Due(at(9, 3, 59), daily, "") {
		t.Fatal("due before the scheduled hour")
	}
	if Due(at(9, 4, 0), daily, "2026-10-09") {
		t.Fatal("due twice on the same day")
	}
	if !Due(at(9, 4, 0), daily, "2026-10-08") {
		t.Fatal("not due on a new day")
	}
	if Due(at(9, 4, 0), Plan{Frequency: FrequencyDaily, Hour: -1}, "") || Due(at(9, 4, 0), Plan{Frequency: FrequencyDaily, Hour: 24}, "") {
		t.Fatal("skipped or invalid hour was due")
	}
}

func TestDefaultFrequencyIsDaily(t *testing.T) {
	plan := Plan{Hour: 4}
	if !Due(at(9, 4, 0), plan, "") {
		t.Fatal("empty frequency is not due at the hour")
	}
	if Due(at(9, 5, 0), plan, "") {
		t.Fatal("empty frequency is due outside the hour")
	}
	if _, ok := RunKey(at(9, 4, 0), plan); !ok {
		t.Fatal("empty frequency has no run key")
	}
	if _, ok := RunKey(at(9, 4, 0), Plan{Frequency: "monthly", Hour: 4}); ok {
		t.Fatal("unknown frequency must not be due")
	}
}

func TestHourlyDueAtMinuteZeroOncePerHour(t *testing.T) {
	hourly := Plan{Frequency: FrequencyHourly, Hour: 17, Weekday: 3}
	if !Due(at(9, 10, 0), hourly, "") {
		t.Fatal("not due at minute 0")
	}
	if Due(at(9, 10, 1), hourly, "") || Due(at(9, 10, 59), hourly, "") {
		t.Fatal("due at a minute other than 0")
	}
	if Due(at(9, 10, 0), hourly, "2026-10-09T10") {
		t.Fatal("due twice in the same hour")
	}
	if !Due(at(9, 11, 0), hourly, "2026-10-09T10") {
		t.Fatal("not due in the next hour")
	}
	key, _ := RunKey(at(9, 10, 0), hourly)
	if key != "2026-10-09T10" {
		t.Fatalf("hourly key = %q", key)
	}
}

func TestWeeklyDueOnWeekdayAtHour(t *testing.T) {
	sunday3 := Plan{Frequency: FrequencyWeekly, Hour: 3, Weekday: 0}
	if !Due(at(11, 3, 0), sunday3, "") {
		t.Fatal("not due on Sunday at 03:00")
	}
	if !Due(at(11, 3, 45), sunday3, "") {
		t.Fatal("not due at any minute of the scheduled hour")
	}
	if Due(at(10, 3, 0), sunday3, "") {
		t.Fatal("due on Saturday (day before the weekday)")
	}
	if Due(at(12, 3, 0), sunday3, "") {
		t.Fatal("due on Monday (day after the weekday)")
	}
	if Due(at(11, 2, 59), sunday3, "") || Due(at(11, 4, 0), sunday3, "") {
		t.Fatal("due outside the scheduled hour on the weekday")
	}
	if Due(at(11, 3, 0), sunday3, "2026-W41") {
		t.Fatal("due twice in the same week")
	}
	if !Due(at(18, 3, 0), sunday3, "2026-W41") {
		t.Fatal("not due in the next week")
	}
}

func TestWeeklyWeekdayBoundaries(t *testing.T) {
	saturday := Plan{Frequency: FrequencyWeekly, Hour: 23, Weekday: 6}
	if !Due(at(10, 23, 0), saturday, "") {
		t.Fatal("Saturday (6) at 23:00 not due")
	}
	if Due(at(11, 23, 0), saturday, "") {
		t.Fatal("Sunday due for a Saturday schedule")
	}
	monday := Plan{Frequency: FrequencyWeekly, Hour: 0, Weekday: 1}
	if !Due(at(12, 0, 0), monday, "") {
		t.Fatal("Monday (1) at 00:00 not due")
	}
	if Due(at(11, 0, 0), monday, "") {
		t.Fatal("Sunday due for a Monday schedule")
	}
	if Due(at(12, 0, 0), Plan{Frequency: FrequencyWeekly, Hour: 0, Weekday: 7}, "") {
		t.Fatal("invalid weekday 7 was due")
	}
	if Due(at(12, 0, 0), Plan{Frequency: FrequencyWeekly, Hour: 0, Weekday: -1}, "") {
		t.Fatal("invalid weekday -1 was due")
	}
	if Due(at(12, 0, 0), Plan{Frequency: FrequencyWeekly, Hour: 24, Weekday: 1}, "") {
		t.Fatal("invalid hour 24 was due")
	}
}

func TestRunnerStartsTasksOnceAndSkipsDisabledOnes(t *testing.T) {
	now := time.Date(2026, 10, 9, 4, 0, 0, 0, time.Local)
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
	r.Add(Task{Name: "updates", Plan: func(context.Context) Plan { return Plan{Frequency: FrequencyDaily, Hour: 4} }, Run: record("updates")})
	r.Add(Task{Name: "backups", Plan: func(context.Context) Plan { return Plan{Frequency: FrequencyDaily, Hour: -1} }, Run: record("backups")})

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

func TestRunnerHourlyRunsOncePerHourAcrossTicks(t *testing.T) {
	now := time.Date(2026, 10, 9, 10, 0, 0, 0, time.Local)
	r := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.Now = func() time.Time { return now }
	started := make(chan struct{}, 8)
	r.Add(Task{Name: "backups", Plan: func(context.Context) Plan { return Plan{Frequency: FrequencyHourly} }, Run: func(context.Context) { started <- struct{}{} }})

	ctx := context.Background()
	r.tick(ctx)
	now = now.Add(30 * time.Second)
	r.tick(ctx) // still minute 0, same hour
	now = now.Add(time.Minute)
	r.tick(ctx) // minute 1
	now = time.Date(2026, 10, 9, 11, 0, 0, 0, time.Local)
	r.tick(ctx)

	deadline := time.After(2 * time.Second)
	for got := 0; got < 2; {
		select {
		case <-started:
			got++
		case <-deadline:
			t.Fatal("hourly task did not run twice")
		}
	}
	select {
	case <-started:
		t.Fatal("hourly task ran more than once per hour")
	case <-time.After(20 * time.Millisecond):
	}
}
