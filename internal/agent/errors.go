package agent

import "errors"

var (
	ErrNotFound          = errors.New("subagent not found")
	ErrCoordinatorClosed = errors.New("subagent coordinator is closed")
	ErrLiveLimit         = errors.New("maximum live subagents reached")
)
