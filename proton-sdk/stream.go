package protonsdk

import (
	"context"
	"fmt"
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
	call.Arguments = append([]byte(nil), call.Arguments...)
	return Event{Kind: EventToolCall, ToolCall: call}
}
func NewUsageEvent(usage Usage) Event {
	return Event{Kind: EventUsage, Usage: usage}
}
func NewRawEvent(data []byte) Event {
	return Event{Kind: EventRaw, RawData: append([]byte(nil), data...)}
}
func NewFinishEvent(reason FinishReason, metadata ProviderMetadata) Event {
	return Event{Kind: EventFinish, FinishReason: reason, ProviderMetadata: cloneProviderMetadata(metadata)}
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
