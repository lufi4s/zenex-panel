package manage

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// SiteUsage is the disk space one website account uses, in bytes.
type SiteUsage struct {
	User          string `json:"user"`
	FilesBytes    int64  `json:"files_bytes"`
	DatabaseBytes int64  `json:"database_bytes"`
	BackupBytes   int64  `json:"backup_bytes"`
}

// usageTTL is how long a measurement is reused. Measuring walks every file of every website,
// so the panel does not repeat it for each page view.
const usageTTL = 5 * time.Minute

// usageCache holds the last measurement. It is guarded by usageMu.
type usageCache struct {
	mu      sync.Mutex
	usages  map[string]SiteUsage
	takenAt time.Time
}

// SiteUsages returns the disk use of every website, keyed by account name. The helper does the
// measuring; the answer is reused for usageTTL.
func (m *Manager) SiteUsages(ctx context.Context) (map[string]SiteUsage, error) {
	m.usage.mu.Lock()
	defer m.usage.mu.Unlock()
	if m.usage.usages != nil && time.Since(m.usage.takenAt) < usageTTL {
		return m.usage.usages, nil
	}
	out, err := m.Helper.Output(ctx, "usage.sites", nil)
	if err != nil {
		return nil, err
	}
	var rows []SiteUsage
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, fmt.Errorf("storage report from the helper is not valid: %w", err)
	}
	usages := make(map[string]SiteUsage, len(rows))
	for _, r := range rows {
		usages[r.User] = r
	}
	m.usage.usages = usages
	m.usage.takenAt = time.Now()
	return usages, nil
}
