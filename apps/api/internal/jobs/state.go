// Package jobs defines the lifecycle of long-running operations.
// Every provisioning, backup, restore, scan, and migration step is a job.
// The transition table is the single source of truth for what may happen next.
package jobs

import (
	"errors"
	"fmt"
)

type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"    // terminal for this attempt; may be retried
	StateCancelled State = "cancelled" // terminal
	StateDead      State = "dead"      // failed and retries exhausted; needs operator action
)

var ErrInvalidTransition = errors.New("invalid job state transition")

var transitions = map[State][]State{
	StateQueued:    {StateRunning, StateCancelled},
	StateRunning:   {StateSucceeded, StateFailed, StateCancelled},
	StateFailed:    {StateQueued, StateDead},
	StateSucceeded: nil,
	StateCancelled: nil,
	StateDead:      {StateQueued}, // operator-initiated requeue only
}

// CanTransition reports whether from -> to is allowed.
func CanTransition(from, to State) bool {
	for _, next := range transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// Transition returns to if the move from -> to is allowed, otherwise ErrInvalidTransition.
func Transition(from, to State) (State, error) {
	if !CanTransition(from, to) {
		return from, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	return to, nil
}

// NextAfterFailure decides where a failed attempt goes. It retries while attempts
// remain and moves to dead once they are exhausted.
func NextAfterFailure(attempts, maxAttempts int) State {
	if attempts < maxAttempts {
		return StateQueued
	}
	return StateDead
}
