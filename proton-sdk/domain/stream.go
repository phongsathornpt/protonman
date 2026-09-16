package domain

import (
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
	EventTextStart      EventKind = "text_start"
	EventTextDelta      EventKind = "text_delta"
	EventTextEnd        EventKind = "text_end"
	EventReasoningDelta EventKind = "reasoning_delta"
	EventToolCallStart  EventKind = "tool_call_start"
	EventToolCallDelta  EventKind = "tool_call_delta"
	EventToolCallEnd    EventKind = "tool_call_end"
	// EventToolCall carries a complete tool call for agent runtimes that do not
	// need incremental argument rendering. Providers may emit both lifecycle
	// events and this normalized complete event.
	EventToolCall EventKind = "tool_call"
	EventUsage    EventKind = "usage"
	EventRaw      EventKind = "raw"
	EventFinish   EventKind = "finish"
)

type Event struct {
	Kind             EventKind
	Text             string
	ReasoningContent string

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
func NewReasoningDeltaEvent(content string) Event {
	return Event{Kind: EventReasoningDelta, ReasoningContent: content}
}
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
	case EventReasoningDelta:
		if e.ReasoningContent == "" {
			return fmt.Errorf("%w: reasoning content is required", ErrInvalidEvent)
		}
		return nil
	case EventToolCallStart:
		if strings.TrimSpace(e.ToolCallID) == "" {
			return fmt.Errorf("%w: tool call id is required", ErrInvalidEvent)
		}
		if strings.TrimSpace(e.ToolName) == "" {
			return fmt.Errorf("%w: tool name is required", ErrInvalidEvent)
		}
		return nil
	case EventToolCallDelta, EventToolCallEnd:
		if strings.TrimSpace(e.ToolCallID) == "" {
			return fmt.Errorf("%w: tool call id is required", ErrInvalidEvent)
		}
		return nil
	case EventToolCall:
		return e.ToolCall.Validate()
	case EventUsage:
		return e.Usage.Validate()
	case EventRaw:
		if len(e.RawData) == 0 {
			return fmt.Errorf("%w: raw data is required for raw event", ErrInvalidEvent)
		}
		return nil
	case EventFinish:
		if !validFinishReason(e.FinishReason) {
			return fmt.Errorf("%w: unsupported finish reason %q", ErrInvalidEvent, e.FinishReason)
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown event kind %q", ErrInvalidEvent, e.Kind)
	}
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
	ReasoningContent string
	ToolCalls        []ToolCall
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata ProviderMetadata
}

// StepResult is retained for source compatibility with earlier SDK releases.
// Deprecated: use Response.
type StepResult = Response
