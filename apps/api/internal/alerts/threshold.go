package alerts

import (
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/sysinfo"
)

// DefaultCooldown is the minimum time between two alerts for the same metric.
const DefaultCooldown = 30 * time.Minute

// Metric names a monitored resource.
type Metric string

const (
	MetricCPU    Metric = "cpu"
	MetricMemory Metric = "memory"
	MetricDisk   Metric = "disk"
)

// Readings are percentages. A negative value means the reading is unknown and is skipped.
type Readings struct {
	CPU    float64
	Memory float64
	Disk   float64
}

// ReadingsFrom converts a host snapshot into the percentages the thresholds use.
// CPU is the one-minute load average as a percentage of the core count.
func ReadingsFrom(m sysinfo.Metrics) Readings {
	r := Readings{CPU: -1, Memory: -1, Disk: -1}
	if m.CPUCount > 0 {
		r.CPU = m.Load1 / float64(m.CPUCount) * 100
	}
	if m.MemTotalBytes > 0 {
		r.Memory = float64(m.MemUsedBytes) / float64(m.MemTotalBytes) * 100
	}
	if m.DiskTotalBytes > 0 {
		r.Disk = float64(m.DiskUsedBytes) / float64(m.DiskTotalBytes) * 100
	}
	return r
}

// Event is a state change that should be announced.
type Event struct {
	Metric    Metric
	Recovered bool    // false: the metric went over its threshold; true: it dropped back
	Value     float64 // reading that caused the event
	Limit     int     // threshold in percent
}

// Tracker decides which threshold crossings to announce. It keeps, per metric,
// whether the metric is over its limit and whether an alert is currently open.
// An alert is never repeated within the cooldown, but a metric that stays over
// its limit is announced once the cooldown has passed.
type Tracker struct {
	Cooldown time.Duration
	states   map[Metric]*metricState
}

type metricState struct {
	notified  bool      // an alert was sent and no recovery has been sent yet
	lastAlert time.Time // when the last alert was sent
}

// NewTracker returns a tracker with the given cooldown.
func NewTracker(cooldown time.Duration) *Tracker {
	return &Tracker{Cooldown: cooldown, states: map[Metric]*metricState{}}
}

// Evaluate records one set of readings and returns the events to send, in metric order.
func (t *Tracker) Evaluate(now time.Time, r Readings, th Thresholds) []Event {
	var events []Event
	checks := []struct {
		metric Metric
		value  float64
		limit  int
	}{
		{MetricCPU, r.CPU, th.CPU},
		{MetricMemory, r.Memory, th.Memory},
		{MetricDisk, r.Disk, th.Disk},
	}
	for _, c := range checks {
		if c.value < 0 {
			continue
		}
		if ev, ok := t.step(now, c.metric, c.value, c.limit); ok {
			events = append(events, ev)
		}
	}
	return events
}

func (t *Tracker) step(now time.Time, m Metric, value float64, limit int) (Event, bool) {
	st := t.states[m]
	if st == nil {
		st = &metricState{}
		t.states[m] = st
	}
	if value < float64(limit) {
		if !st.notified {
			return Event{}, false
		}
		st.notified = false
		return Event{Metric: m, Recovered: true, Value: value, Limit: limit}, true
	}

	if st.notified {
		return Event{}, false
	}
	if !st.lastAlert.IsZero() && now.Sub(st.lastAlert) < t.Cooldown {
		return Event{}, false
	}
	st.notified = true
	st.lastAlert = now
	return Event{Metric: m, Value: value, Limit: limit}, true
}
