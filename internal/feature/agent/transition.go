package agent

import (
	"fmt"
	"time"
)

// StateTransitionError reports an invalid lifecycle edge.
type StateTransitionError struct {
	From State
	To   State
}

func (e *StateTransitionError) Error() string {
	return fmt.Sprintf("%v: %s -> %s", ErrInvalidTransition, e.From, e.To)
}

func (e *StateTransitionError) Unwrap() error { return ErrInvalidTransition }

func validStateTransition(from, to State) bool {
	if from == to {
		return true
	}
	switch from {
	case StateQueued:
		return to == StateRunning || to == StateCanceling || to == StateCanceled || to == StateFailed || to == StateInterrupted
	case StateRunning:
		return to == StateCanceling || to == StateCompleted || to == StateFailed || to == StateCanceled || to == StateInterrupted
	case StateCanceling:
		return to == StateCompleted || to == StateFailed || to == StateCanceled || to == StateInterrupted
	case StateInterrupted:
		return to == StateResuming
	case StateResuming:
		return to == StateResumed || to == StateInterrupted
	default:
		return false
	}
}
func transitionStatus(status AgentStatus, to State, at time.Time, reason string) (AgentStatus, error) {
	if !validStateTransition(status.State, to) {
		return status, &StateTransitionError{From: status.State, To: to}
	}
	status.State = to
	status.Reason = reason
	switch to {
	case StateRunning:
		if status.StartedAt.IsZero() {
			status.StartedAt = at
		}
	case StateCompleted, StateFailed, StateCanceled, StateInterrupted, StateResumed:
		status.FinishedAt = at
	}
	return status, nil
}
