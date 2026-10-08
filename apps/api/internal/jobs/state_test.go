package jobs

import (
	"errors"
	"testing"
)

func TestTerminalStatesHaveNoExit(t *testing.T) {
	for _, s := range []State{StateSucceeded, StateCancelled} {
		for _, to := range []State{StateQueued, StateRunning, StateSucceeded, StateFailed, StateCancelled, StateDead} {
			if CanTransition(s, to) {
				t.Errorf("terminal state %s allowed transition to %s", s, to)
			}
		}
	}
}

func TestSucceededCannotBeRerun(t *testing.T) {
	if _, err := Transition(StateSucceeded, StateRunning); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestRunningJobCannotSkipToQueued(t *testing.T) {
	if CanTransition(StateRunning, StateQueued) {
		t.Error("running -> queued must go through failed")
	}
}

func TestNextAfterFailure(t *testing.T) {
	if got := NextAfterFailure(1, 3); got != StateQueued {
		t.Errorf("attempt 1/3 = %s, want queued", got)
	}
	if got := NextAfterFailure(3, 3); got != StateDead {
		t.Errorf("attempt 3/3 = %s, want dead", got)
	}
}
