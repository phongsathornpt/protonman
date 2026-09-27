package questionbridge

import (
	"context"
	"errors"
	"fmt"
	"sync"

	tea "charm.land/bubbletea/v2"

	questiontool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/question"
)

// Request wraps an interactive question request with a response delivery channel.
type Request struct {
	Request  questiontool.Request
	Response chan Response
}

// Respond delivers the response (and optional error) to the blocking caller.
func (r *Request) Respond(res questiontool.Response, err ...error) {
	if r == nil || r.Response == nil {
		return
	}
	var resErr error
	if len(err) > 0 {
		resErr = err[0]
	}
	r.Response <- Response{Response: res, Err: resErr}
}

// Response holds the resolution returned from the UI to the waiting tool runner.
type Response struct {
	Response questiontool.Response
	Err      error
}

// RequestMsg carries a question request into Bubble Tea's event loop.
type RequestMsg struct {
	Request Request
}

// ClosedMsg indicates the question bridge has closed.
type ClosedMsg struct{}

// Bridge mediates between the blocking worker thread and the Bubble Tea UI event loop.
type Bridge struct {
	requests chan Request
	done     chan struct{}
	once     sync.Once
}

var _ questiontool.Prompter = (*Bridge)(nil)

// New creates a new question bridge.
func New() *Bridge {
	return &Bridge{
		requests: make(chan Request),
		done:     make(chan struct{}),
	}
}

// PromptQuestion blocks waiting for the Bubble Tea UI to resolve the request.
func (b *Bridge) PromptQuestion(ctx context.Context, request questiontool.Request) (questiontool.Response, error) {
	response := make(chan Response, 1)
	pending := Request{Request: request, Response: response}
	select {
	case b.requests <- pending:
	case <-ctx.Done():
		return questiontool.Response{}, fmt.Errorf("question prompt canceled: %w", ctx.Err())
	case <-b.done:
		return questiontool.Response{}, errors.New("question prompt closed")
	}
	select {
	case result := <-response:
		return result.Response, result.Err
	case <-ctx.Done():
		return questiontool.Response{}, fmt.Errorf("question prompt canceled: %w", ctx.Err())
	case <-b.done:
		return questiontool.Response{}, errors.New("question prompt closed")
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
