package manage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
)

// scanTimeout bounds one malware scan, including the comparison with WordPress.org.
const scanTimeout = 20 * time.Minute

// Finding is one suspicious file or line found by the malware scanner. It never holds file text.
type Finding struct {
	Path     string `json:"path"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Line     int    `json:"line,omitempty"`
	Source   string `json:"source"`
}

// ScanReport is the result of one scan.
type ScanReport struct {
	ScannedAt    time.Time `json:"scanned_at"`
	DurationMS   int64     `json:"duration_ms"`
	FilesScanned int       `json:"files_scanned"`
	Findings     []Finding `json:"findings"`
	ChecksumNote string    `json:"checksum_note,omitempty"`
	Truncated    bool      `json:"truncated,omitempty"`
}

// QuarantineItem is a file that was moved out of a website.
type QuarantineItem struct {
	ID            string    `json:"id"`
	Path          string    `json:"path"`
	Rule          string    `json:"rule,omitempty"`
	Size          int64     `json:"size"`
	SHA256        string    `json:"sha256"`
	QuarantinedAt time.Time `json:"quarantined_at"`
}

// SecurityStatus is what the Security tab shows for one website.
type SecurityStatus struct {
	Running    bool             `json:"running"`
	StartedAt  *time.Time       `json:"started_at,omitempty"`
	LastError  string           `json:"last_error,omitempty"`
	Report     *ScanReport      `json:"report"`
	Quarantine []QuarantineItem `json:"quarantine"`
}

// scanState remembers scans that run or failed. It is in memory only: the last finished report
// is kept by the helper, and a panel restart simply ends a running scan.
type scanState struct {
	mu      sync.Mutex
	running map[string]time.Time
	failed  map[string]string
}

func (s *scanState) start(siteID string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running == nil {
		s.running, s.failed = map[string]time.Time{}, map[string]string{}
	}
	if _, busy := s.running[siteID]; busy {
		return false
	}
	s.running[siteID] = now
	delete(s.failed, siteID)
	return true
}

func (s *scanState) finish(siteID, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, siteID)
	if errMsg != "" {
		s.failed[siteID] = errMsg
	}
}

func (s *scanState) get(siteID string) (*time.Time, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.running[siteID]; ok {
		return &t, ""
	}
	return nil, s.failed[siteID]
}

func requireFilesReady(site store.Site) error {
	if site.State != "ready" && site.State != "suspended" {
		return refuse("the scanner works once the website is ready (it is %s)", site.State)
	}
	return nil
}

// StartScan begins a malware scan in the background. Only one scan runs per website.
func (m *Manager) StartScan(_ context.Context, site store.Site) error {
	if err := requireFilesReady(site); err != nil {
		return err
	}
	if !m.scans.start(site.ID, time.Now()) {
		return refuse("a scan of this website is already running")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()
		_, err := m.Helper.Do(ctx, "malware.scan", map[string]string{"user": site.LinuxUser})
		if err != nil {
			m.Log.Warn("malware scan failed", "site_id", site.ID, "error", err)
			m.scans.finish(site.ID, "The scan could not be completed. Try again.")
			return
		}
		m.scans.finish(site.ID, "")
	}()
	return nil
}

// SecurityStatus returns the running state, the last report and the quarantined files.
func (m *Manager) SecurityStatus(ctx context.Context, site store.Site) (SecurityStatus, error) {
	if err := requireFilesReady(site); err != nil {
		return SecurityStatus{}, err
	}
	st := SecurityStatus{Quarantine: []QuarantineItem{}}
	st.StartedAt, st.LastError = m.scans.get(site.ID)
	st.Running = st.StartedAt != nil

	args := map[string]string{"user": site.LinuxUser}
	rep, err := m.Helper.Output(ctx, "malware.report", args)
	if err != nil {
		return SecurityStatus{}, err
	}
	if strings.TrimSpace(rep) != "" {
		var r ScanReport
		if json.Unmarshal([]byte(rep), &r) == nil {
			if r.Findings == nil {
				r.Findings = []Finding{}
			}
			st.Report = &r
		}
	}
	list, err := m.Helper.Output(ctx, "malware.list", args)
	if err != nil {
		return SecurityStatus{}, err
	}
	if strings.TrimSpace(list) != "" {
		if json.Unmarshal([]byte(list), &st.Quarantine) != nil || st.Quarantine == nil {
			st.Quarantine = []QuarantineItem{}
		}
	}
	return st, nil
}

// Quarantine moves one file out of the website. The file is kept and can be restored.
func (m *Manager) Quarantine(ctx context.Context, site store.Site, path, rule string) error {
	if err := requireFilesReady(site); err != nil {
		return err
	}
	if path == "" {
		return errors.New("a file is required")
	}
	_, err := m.Helper.Do(ctx, "malware.quarantine", map[string]string{"user": site.LinuxUser, "path": path, "rule": rule})
	return err
}

// RestoreQuarantined puts a quarantined file back where it was.
func (m *Manager) RestoreQuarantined(ctx context.Context, site store.Site, id string) error {
	if err := requireFilesReady(site); err != nil {
		return err
	}
	_, err := m.Helper.Do(ctx, "malware.restore", map[string]string{"user": site.LinuxUser, "id": id})
	return err
}

// DeleteQuarantined removes a quarantined file for good.
func (m *Manager) DeleteQuarantined(ctx context.Context, site store.Site, id string) error {
	if err := requireFilesReady(site); err != nil {
		return err
	}
	_, err := m.Helper.Do(ctx, "malware.purge", map[string]string{"user": site.LinuxUser, "id": id})
	return err
}
