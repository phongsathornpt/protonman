package permissionbridge

import (
	"context"
	"errors"
	"fmt"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

// Request wraps a permission request with a response delivery channel.
type Request struct {
	Request  permission.Request
	Response chan Response
}

// Respond delivers the resolution (and optional error) to the blocking caller.
func (r *Request) Respond(res permission.Resolution, err ...error) {
	if r == nil || r.Response == nil {
		return
	}
	var resErr error
	if len(err) > 0 {
		resErr = err[0]
	}
	r.Response <- Response{Resolution: res, Err: resErr}
}

// Response holds the resolution returned from the UI to the waiting tool runner.
type Response struct {
	Resolution permission.Resolution
	Err        error
}

// RequestMsg carries a permission request into Bubble Tea's event loop.
type RequestMsg struct {
	Request Request
}

// ClosedMsg indicates the bridge has closed.
type ClosedMsg struct{}

// Bridge mediates between the blocking worker thread and the Bubble Tea UI event loop.
type Bridge struct {
	requests chan Request
	done     chan struct{}
	once     sync.Once
}

// New creates a new permission bridge.
func New() *Bridge {
	return &Bridge{
		requests: make(chan Request),
		done:     make(chan struct{}),
	}
}

// Prompt blocks waiting for the Bubble Tea UI to resolve the request.
func (b *Bridge) Prompt(ctx context.Context, request permission.Request) (permission.Resolution, error) {
	response := make(chan Response, 1)
	pending := Request{Request: request, Response: response}
	select {
	case b.requests <- pending:
	case <-ctx.Done():
		return permission.Resolution{}, fmt.Errorf("permission prompt canceled: %w", ctx.Err())
	case <-b.done:
		return permission.Resolution{}, errors.New("permission prompt closed")
	}
	select {
	case result := <-response:
		return result.Resolution, result.Err
	case <-ctx.Done():
		return permission.Resolution{}, fmt.Errorf("permission prompt canceled: %w", ctx.Err())
	case <-b.done:
		return permission.Resolution{}, errors.New("permission prompt closed")
	}
}

// Next produces a Bubble Tea command that waits for the next incoming prompt request.
func (b *Bridge) Next() tea.Cmd {
	return func() tea.Msg {
		select {
		case request := <-b.requests:
			return RequestMsg{Request: request}
		case <-b.done:
			return ClosedMsg{}
		}
	}
}

// Close closes the bridge, canceling any pending or future prompts.
func (b *Bridge) Close() {
	b.once.Do(func() {
		close(b.done)
	})
}
