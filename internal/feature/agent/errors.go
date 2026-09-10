package agent

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrNotFound          = errors.New("subagent not found")
	ErrCoordinatorClosed = errors.New("subagent coordinator is closed")
	ErrSubagentsDisabled = errors.New("subagents are disabled")
	ErrLiveLimit         = errors.New("maximum live subagents reached")
	ErrNotResumable      = errors.New("subagent is not resumable")
	ErrInvalidTransition = errors.New("invalid subagent lifecycle transition")
	ErrShutdownTimeout   = errors.New("subagent coordinator shutdown timed out")
	ErrQueueTimeout      = errors.New("subagent queue timed out")
	ErrExecutionTimeout  = errors.New("subagent execution timed out")
)

type ShutdownTimeoutError struct {
	ActiveAgents int
}

func (e *ShutdownTimeoutError) Error() string {
	return fmt.Sprintf("%v with %d active subagent(s)", ErrShutdownTimeout, e.ActiveAgents)
}

func (e *ShutdownTimeoutError) Unwrap() error { return ErrShutdownTimeout }

func queueTimeoutError() error {
	return fmt.Errorf("%w: %w", ErrQueueTimeout, context.DeadlineExceeded)
}

func executionTimeoutError() error {
	return fmt.Errorf("%w: %w", ErrExecutionTimeout, context.DeadlineExceeded)
}
