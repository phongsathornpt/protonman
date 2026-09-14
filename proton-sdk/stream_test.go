package protonsdk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type eventStream struct {
	events     []Event
	index      int
	nextErr    error
	closeErr   error
	closeCalls int
}

func (s *eventStream) Next(context.Context) (Event, error) {
	if s.index >= len(s.events) {
		if s.nextErr != nil {
			return Event{}, s.nextErr
		}
		return Event{}, io.EOF
	}
	e := s.events[s.index]
	s.index++
	return e, nil
}

func (s *eventStream) Close() error {
	s.closeCalls++
	return s.closeErr
}

func TestAgentStreamEventLifecycle(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		wantErr bool
	}{
		{name: "text start", event: Event{Kind: EventTextStart}},
		{name: "text delta", event: Event{Kind: EventTextDelta, Text: "hi"}},
		{name: "text end", event: Event{Kind: EventTextEnd}},
		{name: "tool start", event: Event{Kind: EventToolCallStart, ToolCallID: "call-1", ToolName: "read"}},
		{name: "tool delta", event: Event{Kind: EventToolCallDelta, ToolCallID: "call-1", ArgumentsDelta: `{"path"`}},
		{name: "tool end", event: Event{Kind: EventToolCallEnd, ToolCallID: "call-1"}},
		{name: "complete tool call", event: Event{Kind: EventToolCall, ToolCall: ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
		{name: "tool start missing id", event: Event{Kind: EventToolCallStart, ToolName: "read"}, wantErr: true},
		{name: "tool start missing name", event: Event{Kind: EventToolCallStart, ToolCallID: "call-1"}, wantErr: true},
		{name: "missing tool id in delta", event: Event{Kind: EventToolCallDelta}, wantErr: true},
		{name: "missing tool id in end", event: Event{Kind: EventToolCallEnd}, wantErr: true},
		{name: "unsupported event kind", event: Event{Kind: "unsupported_kind"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("Validate() error = nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestRawEventValidation(t *testing.T) {
	if err := (Event{Kind: EventRaw, RawData: []byte(`{"ok":true}`)}).Validate(); err != nil {
		t.Fatalf("raw event error = %v", err)
	}
	if err := (Event{Kind: EventRaw}).Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("empty raw event error = %v", err)
	}
}

func TestUsageAndFinishEvents(t *testing.T) {
	if err := (Event{Kind: EventUsage, Usage: Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12, CachedInputTokens: 5}}).Validate(); err != nil {
		t.Fatalf("usage event error = %v", err)
	}
	if err := (Event{Kind: EventUsage, Usage: Usage{InputTokens: -1}}).Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("negative usage error = %v", err)
	}
	if err := (Event{Kind: EventUsage, Usage: Usage{CachedInputTokens: -1}}).Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("negative cached usage error = %v", err)
	}
	if err := (Event{Kind: EventFinish, FinishReason: FinishToolCalls}).Validate(); err != nil {
		t.Fatalf("finish event error = %v", err)
	}
	if err := (Event{Kind: EventFinish, FinishReason: "provider_magic"}).Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("unknown finish error = %v", err)
	}
}

func TestStreamConstructorsOwnMutablePayloads(t *testing.T) {
	arguments := json.RawMessage(`{"path":"README.md"}`)
	callEvent := NewToolCallEvent(ToolCall{ID: "call-1", Name: "read", Arguments: arguments})
	arguments[0] = '['
	if string(callEvent.ToolCall.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("tool call arguments mutated after construction: %s", callEvent.ToolCall.Arguments)
	}

	raw := []byte(`{"delta":"hello"}`)
	rawEvent := NewRawEvent(raw)
	raw[0] = '['
	if string(rawEvent.RawData) != `{"delta":"hello"}` {
		t.Fatalf("raw event mutated after construction: %s", rawEvent.RawData)
	}

	metadata := ProviderMetadata{"provider": json.RawMessage(`{"id":"x"}`)}
	finishEvent := NewFinishEvent(FinishStop, metadata)
	metadata["provider"][0] = '['
	if string(finishEvent.ProviderMetadata["provider"]) != `{"id":"x"}` {
		t.Fatalf("finish metadata mutated after construction: %s", finishEvent.ProviderMetadata["provider"])
	}
}

func TestCollectStep(t *testing.T) {
	stream := &eventStream{events: []Event{
		NewTextStartEvent(),
		NewTextDeltaEvent("hello "),
		NewTextDeltaEvent("world"),
		NewToolCallEvent(ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}),
		NewFinishEvent(FinishStop, nil),
	}}
	result, err := CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatalf("CollectStep() error = %v", err)
	}
	if result.Text != "hello world" {
		t.Fatalf("Text = %q", result.Text)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "read" {
		t.Fatalf("ToolCalls = %#v", result.ToolCalls)
	}
	if stream.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", stream.closeCalls)
	}
}

func TestCollectStepRejectsIncompleteStream(t *testing.T) {
	stream := &eventStream{events: []Event{NewTextDeltaEvent("partial")}}
	_, err := CollectStep(context.Background(), stream)
	if !errors.Is(err, ErrIncompleteStream) {
		t.Fatalf("error = %v, want ErrIncompleteStream", err)
	}
	if stream.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", stream.closeCalls)
	}
}

func TestCollectStepReturnsCloseErrorAfterSuccessfulFinish(t *testing.T) {
	closeErr := errors.New("close failed")
	stream := &eventStream{events: []Event{NewFinishEvent(FinishStop, nil)}, closeErr: closeErr}
	_, err := CollectStep(context.Background(), stream)
	if !errors.Is(err, closeErr) {
		t.Fatalf("error = %v, want close error", err)
	}
	if stream.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", stream.closeCalls)
	}
}

func TestCollectStepPreservesProcessingErrorOverCloseError(t *testing.T) {
	nextErr := errors.New("next failed")
	closeErr := errors.New("close failed")
	stream := &eventStream{nextErr: nextErr, closeErr: closeErr}
	_, err := CollectStep(context.Background(), stream)
	if !errors.Is(err, nextErr) {
		t.Fatalf("error = %v, want processing error", err)
	}
	if errors.Is(err, closeErr) {
		t.Fatalf("processing error was replaced by close error: %v", err)
	}
	if stream.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", stream.closeCalls)
	}
}

func TestCollectStepIncludesUsageAndFinishReason(t *testing.T) {
	stream := &eventStream{events: []Event{
		NewTextDeltaEvent("done"),
		NewUsageEvent(Usage{InputTokens: 4, OutputTokens: 1, TotalTokens: 5}),
		NewFinishEvent(FinishStop, nil),
	}}
	result, err := CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatalf("CollectStep() error = %v", err)
	}
	if result.FinishReason != FinishStop {
		t.Fatalf("FinishReason = %q", result.FinishReason)
	}
	if result.Usage.TotalTokens != 5 {
		t.Fatalf("Usage = %#v", result.Usage)
	}
}

func TestCollectReturnsCanonicalResponse(t *testing.T) {
	stream := &eventStream{events: []Event{
		NewTextDeltaEvent("hello"),
		NewFinishEvent(FinishStop, nil),
	}}

	response, err := Collect(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "hello" || response.FinishReason != FinishStop {
		t.Fatalf("response = %#v", response)
	}
}

func TestAppendAssistantResponsePreservesToolCalls(t *testing.T) {
	response := Response{
		Text:      "done",
		ToolCalls: []ToolCall{{ID: "call-1", Name: "read"}},
	}
	messages := AppendAssistantResponse(nil, response)
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	if messages[0].Content != "done" || len(messages[0].ToolCalls) != 1 {
		t.Fatalf("assistant message = %#v", messages[0])
	}
}

func TestCollectStepCopiesProviderMetadata(t *testing.T) {
	metadata := ProviderMetadata{"anthropic": json.RawMessage(`{"stop_sequence":"done"}`)}
	stream := &eventStream{events: []Event{{Kind: EventFinish, FinishReason: FinishStop, ProviderMetadata: metadata}}}
	result, err := CollectStep(t.Context(), stream)
	if err != nil {
		t.Fatal(err)
	}
	result.ProviderMetadata["anthropic"][0] = 'X'
	if string(metadata["anthropic"]) != `{"stop_sequence":"done"}` {
		t.Fatalf("source metadata mutated: %s", metadata["anthropic"])
	}
}

func TestResponseAccumulatorBuildsCanonicalResponse(t *testing.T) {
	metadata := ProviderMetadata{"provider": json.RawMessage(`{"request_id":"req_1"}`)}
	var accumulator ResponseAccumulator
	for _, event := range []Event{
		NewTextStartEvent(),
		NewTextDeltaEvent("hello "),
		NewTextDeltaEvent("world"),
		NewToolCallEvent(ToolCall{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}),
		NewUsageEvent(Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}),
		NewFinishEvent(FinishStop, metadata),
	} {
		if err := accumulator.Absorb(event); err != nil {
			t.Fatalf("Absorb(%q) error = %v", event.Kind, err)
		}
	}

	response, err := accumulator.Finish()
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if response.Text != "hello world" || response.FinishReason != FinishStop {
		t.Fatalf("response = %#v", response)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Name != "read" {
		t.Fatalf("ToolCalls = %#v", response.ToolCalls)
	}
	if response.Usage.TotalTokens != 6 {
		t.Fatalf("Usage = %#v", response.Usage)
	}
	if string(response.ProviderMetadata["provider"]) != `{"request_id":"req_1"}` {
		t.Fatalf("ProviderMetadata = %#v", response.ProviderMetadata)
	}

	metadata["provider"][0] = '['
	if string(response.ProviderMetadata["provider"]) != `{"request_id":"req_1"}` {
		t.Fatal("response metadata aliases finish-event metadata")
	}
}

func TestResponseAccumulatorRequiresTerminalFinish(t *testing.T) {
	var accumulator ResponseAccumulator
	if err := accumulator.Absorb(NewTextDeltaEvent("partial")); err != nil {
		t.Fatal(err)
	}
	if _, err := accumulator.Finish(); !errors.Is(err, ErrIncompleteStream) {
		t.Fatalf("Finish() error = %v, want ErrIncompleteStream", err)
	}
}

func TestResponseAccumulatorRejectsEventsAfterFinish(t *testing.T) {
	var accumulator ResponseAccumulator
	if err := accumulator.Absorb(NewFinishEvent(FinishStop, nil)); err != nil {
		t.Fatal(err)
	}
	if err := accumulator.Absorb(NewTextDeltaEvent("late")); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("Absorb() error = %v, want ErrInvalidEvent", err)
	}
}

func TestResponseAccumulatorFinishReturnsOwnedPayloads(t *testing.T) {
	arguments := json.RawMessage(`{"path":"README.md"}`)
	var accumulator ResponseAccumulator
	if err := accumulator.Absorb(NewToolCallEvent(ToolCall{ID: "call_1", Name: "read", Arguments: arguments})); err != nil {
		t.Fatal(err)
	}
	if err := accumulator.Absorb(NewFinishEvent(FinishStop, nil)); err != nil {
		t.Fatal(err)
	}

	first, err := accumulator.Finish()
	if err != nil {
		t.Fatal(err)
	}
	first.ToolCalls[0].Arguments[0] = '['

	second, err := accumulator.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if string(second.ToolCalls[0].Arguments) != `{"path":"README.md"}` {
		t.Fatal("Finish() returned aliases to accumulator-owned tool arguments")
	}
}

func TestGranularEventConstructors(t *testing.T) {
	if got := NewTextEndEvent(); got.Kind != EventTextEnd {
		t.Fatalf("NewTextEndEvent() = %+v", got)
	}
	start := NewToolCallStartEvent("call-1", "read")
	if start.Kind != EventToolCallStart || start.ToolCallID != "call-1" || start.ToolName != "read" {
		t.Fatalf("NewToolCallStartEvent() = %+v", start)
	}
	delta := NewToolCallDeltaEvent("call-1", `{"path"`)
	if delta.Kind != EventToolCallDelta || delta.ToolCallID != "call-1" || delta.ArgumentsDelta != `{"path"` {
		t.Fatalf("NewToolCallDeltaEvent() = %+v", delta)
	}
	end := NewToolCallEndEvent("call-1")
	if end.Kind != EventToolCallEnd || end.ToolCallID != "call-1" {
		t.Fatalf("NewToolCallEndEvent() = %+v", end)
	}
}

func TestCollectRejectsNilStream(t *testing.T) {
	if _, err := Collect(context.Background(), nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Collect(nil) error = %v, want ErrInvalidRequest", err)
	}
}

func TestCollectRejectsInvalidEventInStream(t *testing.T) {
	stream := &eventStream{events: []Event{{Kind: "bad_kind"}}}
	if _, err := Collect(context.Background(), stream); err == nil {
		t.Fatal("expected error on stream with invalid event")
	}
	if stream.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", stream.closeCalls)
	}
}

func TestResponseAccumulatorNilSafety(t *testing.T) {
	var accumulator *ResponseAccumulator
	if err := accumulator.Absorb(NewTextDeltaEvent("text")); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("nil Absorb() error = %v, want ErrInvalidEvent", err)
	}
	if _, err := accumulator.Finish(); !errors.Is(err, ErrIncompleteStream) {
		t.Fatalf("nil Finish() error = %v, want ErrIncompleteStream", err)
	}
}

func TestResponseAccumulatorRejectsInvalidEvent(t *testing.T) {
	var accumulator ResponseAccumulator
	if err := accumulator.Absorb(Event{Kind: "invalid"}); err == nil {
		t.Fatal("expected error on invalid event")
	}
}

