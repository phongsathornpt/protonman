package agent

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound          = errors.New("subagent not found")
	ErrCoordinatorClosed = errors.New("subagent coordinator is closed")
	ErrSubagentsDisabled = errors.New("subagents are disabled")
	ErrLiveLimit         = errors.New("maximum live subagents reached")
	ErrNotResumable      = errors.New("subagent is not resumable")
	ErrShutdownTimeout   = errors.New("subagent coordinator shutdown timed out")
)

type ShutdownTimeoutError struct {
	ActiveAgents int
}

func (e *ShutdownTimeoutError) Error() string {
	return fmt.Sprintf("%v with %d active subagent(s)", ErrShutdownTimeout, e.ActiveAgents)
}

func (e *ShutdownTimeoutError) Unwrap() error { return ErrShutdownTimeout }
