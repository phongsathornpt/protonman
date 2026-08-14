// Package turn coordinates one model response stream with permission-aware
// tool execution.
package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/projectTHORN/proton/internal/application/toolcall"
	"github.com/projectTHORN/proton/internal/domain/model"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

const defaultMaxRounds = 8

var (
	// ErrInvalidLoop indicates that the loop cannot be constructed or started.
	ErrInvalidLoop = errors.New("invalid model/tool loop")
	// ErrMaxRounds indicates that a model kept requesting tools without a final response.
	ErrMaxRounds = errors.New("model/tool round limit exceeded")
)

// EventKind identifies progress emitted by the application loop.
type EventKind string

const (
	// EventTextDelta forwards streamed model text.
	EventTextDelta EventKind = "text_delta"
	// EventToolCall reports a validated tool call before permission evaluation.
	EventToolCall EventKind = "tool_call"
	// EventToolResult reports the terminal result of a permission-aware call.
	EventToolResult EventKind = "tool_result"
	// EventCompleted marks a final model response with no further tool calls.
	EventCompleted EventKind = "completed"
	// EventFailed reports a terminal loop failure.
	EventFailed EventKind = "failed"
)

// Event is one progress or terminal notification from a model/tool turn.
type Event struct {
	Kind    EventKind
	Round   int
	Text    string
	Call    tool.Call
	Result  tool.Result
	Message model.Message
	Err     error
}

// Sink receives loop events in emission order.
type Sink func(context.Context, Event) error

// Result is the final assistant response from a completed turn.
type Result struct {
	Message model.Message
	Rounds  int
}

// Option configures a Loop.
type Option func(*Loop) error

// WithMaxRounds bounds model responses that can request more tools.
func WithMaxRounds(rounds int) Option {
	return func(loop *Loop) error {
		if rounds <= 0 {
			return fmt.Errorf("%w: max rounds must be positive", ErrInvalidLoop)
		}
		loop.maxRounds = rounds
		return nil
	}
}

// Loop coordinates model streaming and permission-aware tool dispatch.
type Loop struct {
	client    model.Client
	tools     *toolcall.Service
	maxRounds int
}

// NewLoop creates a provider-neutral model/tool execution loop.
func NewLoop(client model.Client, tools *toolcall.Service, options ...Option) (*Loop, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: model client is required", ErrInvalidLoop)
	}
	if tools == nil {
		return nil, fmt.Errorf("%w: tool-call service is required", ErrInvalidLoop)
	}
	loop := &Loop{
		client:    client,
		tools:     tools,
		maxRounds: defaultMaxRounds,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(loop); err != nil {
			return nil, err
		}
	}
	return loop, nil
}

// Run executes model responses until one has no tool calls or the round bound is reached.
func (l *Loop) Run(ctx context.Context, messages []model.Message, sink Sink) (Result, error) {
	if l == nil {
		return Result{}, fmt.Errorf("%w: loop is required", ErrInvalidLoop)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("start model/tool loop: %w", err)
	}
	if sink == nil {
		sink = func(context.Context, Event) error { return nil }
	}

	history := model.CloneMessages(messages)
	for round := 1; round <= l.maxRounds; round++ {
		request := model.Request{
			Messages: model.CloneMessages(history),
			Tools:    l.tools.Definitions(),
		}
		if err := request.Validate(); err != nil {
			return l.fail(ctx, sink, round, err)
		}
		assistant, calls, err := l.streamRound(ctx, round, request, sink)
		if err != nil {
			return Result{}, err
		}
		history = append(history, assistant)
		if len(calls) == 0 {
			result := Result{
				Message: assistant,
				Rounds:  round,
			}
			if err := emit(ctx, sink, Event{
				Kind:    EventCompleted,
				Round:   round,
				Message: assistant,
			}); err != nil {
				return Result{}, err
			}
			return result, nil
		}

		for _, requestedCall := range calls {
			call, err := tool.NewCall(
				requestedCall.ID,
				requestedCall.Name,
				requestedCall.Arguments,
			)
			if err != nil {
				return l.fail(ctx, sink, round, fmt.Errorf("translate model tool call: %w", err))
			}
			if err := emit(ctx, sink, Event{
				Kind:  EventToolCall,
				Round: round,
				Call:  call,
			}); err != nil {
				return Result{}, err
			}

			toolResult, callErr := l.tools.Call(ctx, call)
			if callErr != nil && toolResult.Failure == nil {
				toolResult.Failure = tool.FailureFromError(callErr)
			}
			if err := emit(ctx, sink, Event{
				Kind:   EventToolResult,
				Round:  round,
				Call:   call,
				Result: toolResult,
				Err:    callErr,
			}); err != nil {
				return Result{}, err
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Result{}, fmt.Errorf("after tool call %q: %w", call.Name, ctxErr)
			}
			content, err := json.Marshal(toolResult)
			if err != nil {
				return l.fail(ctx, sink, round, fmt.Errorf("encode tool result %q: %w", call.Name, err))
			}
			history = append(history, model.Message{
				Role:       model.RoleTool,
				Content:    string(content),
				ToolCallID: call.ID,
				ToolName:   call.Name,
			})
		}
	}

	return l.fail(ctx, sink, l.maxRounds, fmt.Errorf("%w: %d rounds", ErrMaxRounds, l.maxRounds))
}

func (l *Loop) streamRound(
	ctx context.Context,
	round int,
	request model.Request,
	sink Sink,
) (model.Message, []model.ToolCall, error) {
	stream, err := l.client.Stream(ctx, request)
	if err != nil {
		return model.Message{}, nil, fmt.Errorf("stream model round %d: %w", round, err)
	}
	if stream == nil {
		return model.Message{}, nil, fmt.Errorf("stream model round %d: nil stream", round)
	}
	assistant, calls, streamErr := consumeStream(ctx, round, stream, sink)
	closeErr := stream.Close()
	if streamErr != nil {
		return model.Message{}, nil, streamErr
	}
	if closeErr != nil {
		return model.Message{}, nil, fmt.Errorf("close model stream round %d: %w", round, closeErr)
	}
	return assistant, calls, nil
}

func consumeStream(
	ctx context.Context,
	round int,
	stream model.Stream,
	sink Sink,
) (model.Message, []model.ToolCall, error) {
	var text strings.Builder
	calls := make([]model.ToolCall, 0)
	for {
		event, err := stream.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return model.Message{}, nil, fmt.Errorf("read model stream round %d: %w", round, err)
		}
		if err := event.Validate(); err != nil {
			return model.Message{}, nil, fmt.Errorf("validate model stream round %d: %w", round, err)
		}
		switch event.Kind {
		case model.EventTextDelta:
			text.WriteString(event.Text)
			if err := emit(ctx, sink, Event{
				Kind:  EventTextDelta,
				Round: round,
				Text:  event.Text,
			}); err != nil {
				return model.Message{}, nil, err
			}
		case model.EventToolCall:
			call := event.ToolCall
			call.Arguments = append(json.RawMessage{}, call.Arguments...)
			calls = append(calls, call)
		case model.EventDone:
			return model.Message{
				Role:      model.RoleAssistant,
				Content:   text.String(),
				ToolCalls: calls,
			}, calls, nil
		}
	}
	return model.Message{
		Role:      model.RoleAssistant,
		Content:   text.String(),
		ToolCalls: calls,
	}, calls, nil
}

func (l *Loop) fail(ctx context.Context, sink Sink, round int, err error) (Result, error) {
	if emitErr := emit(ctx, sink, Event{
		Kind:  EventFailed,
		Round: round,
		Err:   err,
	}); emitErr != nil {
		return Result{}, emitErr
	}
	return Result{}, err
}

func emit(ctx context.Context, sink Sink, event Event) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("emit model/tool event: %w", err)
	}
	if err := sink(ctx, event); err != nil {
		return fmt.Errorf("emit model/tool event: %w", err)
	}
	return nil
}
