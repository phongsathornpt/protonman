package protonsdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

type FinishReason string

const (
	FinishStop      FinishReason = "stop"
	FinishLength    FinishReason = "length"
	FinishToolCalls FinishReason = "tool_calls"
	FinishError     FinishReason = "error"
	FinishOther     FinishReason = "other"
)

type EventKind string

const (
	EventTextStart     EventKind = "text_start"
	EventTextDelta     EventKind = "text_delta"
	EventTextEnd       EventKind = "text_end"
	EventToolCallStart EventKind = "tool_call_start"
	EventToolCallDelta EventKind = "tool_call_delta"
	EventToolCallEnd   EventKind = "tool_call_end"
	// EventToolCall carries a complete tool call for agent runtimes that do not
	// need incremental argument rendering. Providers may emit both lifecycle
	// events and this normalized complete event.
	EventToolCall EventKind = "tool_call"
	EventUsage    EventKind = "usage"
	EventRaw      EventKind = "raw"
	EventFinish   EventKind = "finish"
)

type Event struct {
	Kind EventKind
	Text string

	ToolCall         ToolCall
	ToolCallID       string
	ToolName         string
	ArgumentsDelta   string
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata ProviderMetadata
	RawData          []byte
}

func NewTextStartEvent() Event { return Event{Kind: EventTextStart} }
func NewTextDeltaEvent(text string) Event {
	return Event{Kind: EventTextDelta, Text: text}
}
func NewTextEndEvent() Event { return Event{Kind: EventTextEnd} }
func NewToolCallStartEvent(id, name string) Event {
	return Event{Kind: EventToolCallStart, ToolCallID: id, ToolName: name}
}
func NewToolCallDeltaEvent(id, argumentsDelta string) Event {
	return Event{Kind: EventToolCallDelta, ToolCallID: id, ArgumentsDelta: argumentsDelta}
}
func NewToolCallEndEvent(id string) Event {
	return Event{Kind: EventToolCallEnd, ToolCallID: id}
}
func NewToolCallEvent(call ToolCall) Event {
	return Event{Kind: EventToolCall, ToolCall: call.Clone()}
}
func NewUsageEvent(usage Usage) Event {
	return Event{Kind: EventUsage, Usage: usage}
}
func NewRawEvent(data []byte) Event {
	return Event{Kind: EventRaw, RawData: append([]byte(nil), data...)}
}
func NewFinishEvent(reason FinishReason, metadata ProviderMetadata) Event {
	return Event{Kind: EventFinish, FinishReason: reason, ProviderMetadata: metadata.Clone()}
}

func (e Event) Validate() error {
	switch e.Kind {
	case EventTextStart, EventTextDelta, EventTextEnd:
		return nil
	case EventRaw:
		if len(e.RawData) == 0 {
			return fmt.Errorf("%w: raw event data is required", ErrInvalidEvent)
		}
		return nil
	case EventUsage:
		return e.Usage.Validate()
	case EventFinish:
		if !validFinishReason(e.FinishReason) {
			return fmt.Errorf("%w: unsupported finish reason %q", ErrInvalidEvent, e.FinishReason)
		}
		return nil
	case EventToolCallStart:
		if strings.TrimSpace(e.ToolCallID) == "" {
			return fmt.Errorf("%w: tool call start id is required", ErrInvalidEvent)
		}
		if strings.TrimSpace(e.ToolName) == "" {
			return fmt.Errorf("%w: tool call start name is required", ErrInvalidEvent)
		}
		return nil
	case EventToolCallDelta, EventToolCallEnd:
		if strings.TrimSpace(e.ToolCallID) == "" {
			return fmt.Errorf("%w: tool call id is required", ErrInvalidEvent)
		}
		return nil
	case EventToolCall:
		return e.ToolCall.Validate()
	default:
		return fmt.Errorf("%w: unsupported event kind %q", ErrInvalidEvent, e.Kind)
	}
}

type Stream interface {
	Next(ctx context.Context) (Event, error)
	Close() error
}

func validFinishReason(reason FinishReason) bool {
	switch reason {
	case FinishStop, FinishLength, FinishToolCalls, FinishError, FinishOther:
		return true
	default:
		return false
	}
}

type Usage struct {
	InputTokens       int64
	OutputTokens      int64
	TotalTokens       int64
	CachedInputTokens int64
}

func (u Usage) Validate() error {
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 || u.CachedInputTokens < 0 {
		return fmt.Errorf("%w: token usage cannot be negative", ErrInvalidEvent)
	}
	return nil
}

// Response is the normalized output collected from one model stream.
type Response struct {
	Text             string
	ToolCalls        []ToolCall
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata ProviderMetadata
}

// StepResult is retained for source compatibility with earlier SDK releases.
// Deprecated: use Response.
type StepResult = Response

// ResponseAccumulator incrementally reconstructs one canonical Response from
// normalized stream events while keeping stream transport concerns separate.
type ResponseAccumulator struct {
	text     strings.Builder
	response Response
	finished bool
}

// Absorb applies one normalized stream event to the response state machine.
// A terminal finish event seals the accumulator; later events are rejected.
func (a *ResponseAccumulator) Absorb(event Event) error {
	if a == nil {
		return fmt.Errorf("%w: response accumulator is required", ErrInvalidEvent)
	}
	if a.finished {
		return fmt.Errorf("%w: response already finished", ErrInvalidEvent)
	}
	if err := event.Validate(); err != nil {
		return err
	}

	switch event.Kind {
	case EventTextDelta:
		a.text.WriteString(event.Text)
	case EventToolCall:
		a.response.ToolCalls = append(a.response.ToolCalls, event.ToolCall.Clone())
	case EventUsage:
		// Usage events are normalized as complete snapshots by the current
		// provider adapters. Keep the latest snapshot until the usage model can
		// represent optional or partial counters explicitly.
		a.response.Usage = event.Usage
	case EventFinish:
		a.response.FinishReason = event.FinishReason
		a.response.ProviderMetadata = event.ProviderMetadata.Clone()
		a.finished = true
	}
	return nil
}

// Finish returns the completed canonical response. Calling Finish before a
// terminal event preserves the SDK's incomplete-stream invariant.
func (a *ResponseAccumulator) Finish() (Response, error) {
	if a == nil || !a.finished {
		return Response{}, ErrIncompleteStream
	}
	response := a.response
	response.Text = a.text.String()
	response.ProviderMetadata = a.response.ProviderMetadata.Clone()
	response.ToolCalls = make([]ToolCall, 0, len(a.response.ToolCalls))
	for _, call := range a.response.ToolCalls {
		response.ToolCalls = append(response.ToolCalls, call.Clone())
	}
	return response, nil
}

// Collect consumes one model stream until its terminal event and builds a
// provider-neutral response through the canonical response accumulator.
func Collect(ctx context.Context, stream Stream) (result Response, err error) {
	if stream == nil {
		return Response{}, fmt.Errorf("%w: stream is required", ErrInvalidRequest)
	}
	defer func() {
		if closeErr := stream.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close model stream: %w", closeErr)
		}
	}()

	var accumulator ResponseAccumulator
	for {
		event, err := stream.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return Response{}, ErrIncompleteStream
			}
			return Response{}, err
		}
		if err := accumulator.Absorb(event); err != nil {
			return Response{}, err
		}
		if event.Kind == EventFinish {
			return accumulator.Finish()
		}
	}
}

// CollectStep is retained for source compatibility with earlier SDK releases.
// Deprecated: use Collect.
func CollectStep(ctx context.Context, stream Stream) (StepResult, error) {
	return Collect(ctx, stream)
}
