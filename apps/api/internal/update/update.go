// Package update reports the installed and latest panel versions and starts
// updates. Every privileged step goes through the root helper.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrRunning is returned when an update is already in progress.
var ErrRunning = errors.New("an update is already running")

// Helper runs fixed operations as root.
type Helper interface {
	Output(ctx context.Context, op string, args map[string]string) (string, error)
	Do(ctx context.Context, op string, args map[string]string) (string, error)
}

// Status is what the Settings page shows about updates.
type Status struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	State           string `json:"state"` // idle, running, succeeded or failed
	Log             string `json:"log"`
	// Changes lists what the newest version adds, newest first. Empty when up to date.
	Changes []Change `json:"changes"`
	// Error explains why the latest version is unknown (for example, no internet).
	Error string `json:"error,omitempty"`
}

// Change is one thing the newest version adds: a short commit id and its title.
type Change struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Service checks for and starts panel updates.
type Service struct {
	helper Helper
}

// New returns a service that uses the helper for every privileged step.
func New(h Helper) *Service {
	return &Service{helper: h}
}

// Status reads the installed version, asks GitHub for the latest one and
// reports the state of the last update.
func (s *Service) Status(ctx context.Context) (Status, error) {
	var st Status

	current, err := s.helper.Output(ctx, "panel.version", map[string]string{})
	if err != nil {
		return st, err
	}
	st.Current = current

	latest, lerr := s.helper.Output(ctx, "panel.latest", map[string]string{})
	if lerr != nil {
		st.Error = lerr.Error()
	} else {
		st.Latest = latest
	}
	st.UpdateAvailable = st.Latest != "" && st.Current != "" && st.Latest != st.Current

	st.Changes = []Change{}
	if st.UpdateAvailable {
		// Best effort: the update works without the list.
		if out, cerr := s.helper.Output(ctx, "panel.changes", map[string]string{}); cerr == nil {
			for _, line := range strings.Split(out, "\n") {
				id, title, ok := strings.Cut(strings.TrimSpace(line), "	")
				if ok && id != "" && title != "" {
					st.Changes = append(st.Changes, Change{ID: id, Title: title})
				}
			}
		}
	}

	raw, err := s.helper.Output(ctx, "panel.update-status", map[string]string{})
	if err != nil {
		return st, err
	}
	var progress struct {
		State string `json:"state"`
		Log   string `json:"log"`
	}
	if err := json.Unmarshal([]byte(raw), &progress); err != nil {
		return st, fmt.Errorf("the helper sent an unreadable update status: %w", err)
	}
	st.State = progress.State
	st.Log = progress.Log
	return st, nil
}

// Start begins an update to the latest version. It returns ErrRunning when one
// is already in progress.
func (s *Service) Start(ctx context.Context) error {
	st, err := s.Status(ctx)
	if err != nil {
		return err
	}
	if st.State == "running" {
		return ErrRunning
	}
	_, err = s.helper.Do(ctx, "panel.update-start", map[string]string{})
	return err
}
