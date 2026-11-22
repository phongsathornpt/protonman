package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/usecase"
)

type eventStream struct {
	events     []domain.Event
	index      int
	nextErr    error
	closeErr   error
	closeCalls int
}

func (s *eventStream) Next(context.Context) (domain.Event, error) {
	if s.index >= len(s.events) {
		if s.nextErr != nil {
			return domain.Event{}, s.nextErr
		}
		return domain.Event{}, io.EOF
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
		event   domain.Event
		wantErr bool
	}{
		{name: "text start", event: domain.Event{Kind: domain.EventTextStart}},
		{name: "text delta", event: domain.Event{Kind: domain.EventTextDelta, Text: "hi"}},
		{name: "text end", event: domain.Event{Kind: domain.EventTextEnd}},
		{name: "tool start", event: domain.Event{Kind: domain.EventToolCallStart, ToolCallID: "call-1", ToolName: "read"}},
		{name: "tool delta", event: domain.Event{Kind: domain.EventToolCallDelta, ToolCallID: "call-1", ArgumentsDelta: `{"path"`}},
		{name: "tool end", event: domain.Event{Kind: domain.EventToolCallEnd, ToolCallID: "call-1"}},
		{name: "complete tool call", event: domain.Event{Kind: domain.EventToolCall, ToolCall: domain.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
		{name: "tool start missing id", event: domain.Event{Kind: domain.EventToolCallStart, ToolName: "read"}, wantErr: true},
		{name: "tool start missing name", event: domain.Event{Kind: domain.EventToolCallStart, ToolCallID: "call-1"}, wantErr: true},
		{name: "missing tool id in delta", event: domain.Event{Kind: domain.EventToolCallDelta}, wantErr: true},
		{name: "missing tool id in end", event: domain.Event{Kind: domain.EventToolCallEnd}, wantErr: true},
		{name: "unsupported event kind", event: domain.Event{Kind: "unsupported_kind"}, wantErr: true},
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
	if err := (domain.Event{Kind: domain.EventRaw, RawData: []byte(`{"ok":true}`)}).Validate(); err != nil {
		t.Fatalf("raw event error = %v", err)
	}
	if err := (domain.Event{Kind: domain.EventRaw}).Validate(); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("empty raw event error = %v", err)
	}
}

func TestUsageAndFinishEvents(t *testing.T) {
	if err := (domain.Event{Kind: domain.EventUsage, Usage: domain.Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12, CachedInputTokens: 5}}).Validate(); err != nil {
		t.Fatalf("usage event error = %v", err)
	}
	if err := (domain.Event{Kind: domain.EventUsage, Usage: domain.Usage{InputTokens: -1}}).Validate(); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("negative usage error = %v", err)
	}
	if err := (domain.Event{Kind: domain.EventUsage, Usage: domain.Usage{CachedInputTokens: -1}}).Validate(); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("negative cached usage error = %v", err)
	}
	if err := (domain.Event{Kind: domain.EventFinish, FinishReason: domain.FinishToolCalls}).Validate(); err != nil {
		t.Fatalf("finish event error = %v", err)
	}
	if err := (domain.Event{Kind: domain.EventFinish, FinishReason: "provider_magic"}).Validate(); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("unknown finish error = %v", err)
	}
}

func TestStreamConstructorsOwnMutablePayloads(t *testing.T) {
	arguments := json.RawMessage(`{"path":"README.md"}`)
	callEvent := domain.NewToolCallEvent(domain.ToolCall{ID: "call-1", Name: "read", Arguments: arguments})
	arguments[0] = '['
	if string(callEvent.ToolCall.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("tool call arguments mutated after construction: %s", callEvent.ToolCall.Arguments)
	}

	raw := []byte(`{"delta":"hello"}`)
	rawEvent := domain.NewRawEvent(raw)
	raw[0] = '['
	if string(rawEvent.RawData) != `{"delta":"hello"}` {
		t.Fatalf("raw event mutated after construction: %s", rawEvent.RawData)
	}

	metadata := domain.ProviderMetadata{"provider": json.RawMessage(`{"id":"x"}`)}
	finishEvent := domain.NewFinishEvent(domain.FinishStop, metadata)
	metadata["provider"][0] = '['
	if string(finishEvent.ProviderMetadata["provider"]) != `{"id":"x"}` {
		t.Fatalf("finish metadata mutated after construction: %s", finishEvent.ProviderMetadata["provider"])
	}
}

func TestCollectStep(t *testing.T) {
	stream := &eventStream{events: []domain.Event{
		domain.NewTextStartEvent(),
		domain.NewTextDeltaEvent("hello "),
		domain.NewTextDeltaEvent("world"),
		domain.NewToolCallEvent(domain.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}),
		domain.NewFinishEvent(domain.FinishStop, nil),
	}}
	result, err := usecase.CollectStep(context.Background(), stream)
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
	stream := &eventStream{events: []domain.Event{domain.NewTextDeltaEvent("partial")}}
	_, err := usecase.CollectStep(context.Background(), stream)
	if !errors.Is(err, domain.ErrIncompleteStream) {
		t.Fatalf("error = %v, want ErrIncompleteStream", err)
	}
	if stream.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", stream.closeCalls)
	}
}

func TestCollectStepReturnsCloseErrorAfterSuccessfulFinish(t *testing.T) {
	closeErr := errors.New("close failed")
	stream := &eventStream{events: []domain.Event{domain.NewFinishEvent(domain.FinishStop, nil)}, closeErr: closeErr}
	_, err := usecase.CollectStep(context.Background(), stream)
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
	_, err := usecase.CollectStep(context.Background(), stream)
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
	stream := &eventStream{events: []domain.Event{
		domain.NewTextDeltaEvent("done"),
		domain.NewUsageEvent(domain.Usage{InputTokens: 4, OutputTokens: 1, TotalTokens: 5}),
		domain.NewFinishEvent(domain.FinishStop, nil),
	}}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatalf("CollectStep() error = %v", err)
	}
	if result.FinishReason != domain.FinishStop {
		t.Fatalf("FinishReason = %q", result.FinishReason)
	}
	if result.Usage.TotalTokens != 5 {
		t.Fatalf("Usage = %#v", result.Usage)
	}
}

func TestCollectReturnsCanonicalResponse(t *testing.T) {
	stream := &eventStream{events: []domain.Event{
		domain.NewTextDeltaEvent("hello"),
		domain.NewFinishEvent(domain.FinishStop, nil),
	}}

	response, err := usecase.Collect(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "hello" || response.FinishReason != domain.FinishStop {
		t.Fatalf("response = %#v", response)
	}
}

func TestAppendAssistantResponsePreservesToolCalls(t *testing.T) {
	response := domain.Response{
		Text:      "done",
		ToolCalls: []domain.ToolCall{{ID: "call-1", Name: "read"}},
	}
	messages := usecase.AppendAssistantResponse(nil, response)
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	if messages[0].Content != "done" || len(messages[0].ToolCalls) != 1 {
		t.Fatalf("assistant message = %#v", messages[0])
	}
}

func TestCollectStepCopiesProviderMetadata(t *testing.T) {
	metadata := domain.ProviderMetadata{"anthropic": json.RawMessage(`{"stop_sequence":"done"}`)}
	stream := &eventStream{events: []domain.Event{{Kind: domain.EventFinish, FinishReason: domain.FinishStop, ProviderMetadata: metadata}}}
	result, err := usecase.CollectStep(t.Context(), stream)
	if err != nil {
		t.Fatal(err)
	}
	result.ProviderMetadata["anthropic"][0] = 'X'
	if string(metadata["anthropic"]) != `{"stop_sequence":"done"}` {
		t.Fatalf("source metadata mutated: %s", metadata["anthropic"])
	}
}

func TestResponseAccumulatorBuildsCanonicalResponse(t *testing.T) {
	metadata := domain.ProviderMetadata{"provider": json.RawMessage(`{"request_id":"req_1"}`)}
	var accumulator usecase.ResponseAccumulator
	for _, event := range []domain.Event{
		domain.NewTextStartEvent(),
		domain.NewTextDeltaEvent("hello "),
		domain.NewTextDeltaEvent("world"),
		domain.NewToolCallEvent(domain.ToolCall{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}),
		domain.NewUsageEvent(domain.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 6}),
		domain.NewFinishEvent(domain.FinishStop, metadata),
	} {
		if err := accumulator.Absorb(event); err != nil {
			t.Fatalf("Absorb(%q) error = %v", event.Kind, err)
		}
	}

	response, err := accumulator.Finish()
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if response.Text != "hello world" || response.FinishReason != domain.FinishStop {
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
	var accumulator usecase.ResponseAccumulator
	if err := accumulator.Absorb(domain.NewTextDeltaEvent("partial")); err != nil {
		t.Fatal(err)
	}
	if _, err := accumulator.Finish(); !errors.Is(err, domain.ErrIncompleteStream) {
		t.Fatalf("Finish() error = %v, want ErrIncompleteStream", err)
	}
}

func TestResponseAccumulatorRejectsEventsAfterFinish(t *testing.T) {
	var accumulator usecase.ResponseAccumulator
	if err := accumulator.Absorb(domain.NewFinishEvent(domain.FinishStop, nil)); err != nil {
		t.Fatal(err)
	}
	if err := accumulator.Absorb(domain.NewTextDeltaEvent("late")); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("Absorb() error = %v, want ErrInvalidEvent", err)
	}
}

func TestResponseAccumulatorFinishReturnsOwnedPayloads(t *testing.T) {
	arguments := json.RawMessage(`{"path":"README.md"}`)
	var accumulator usecase.ResponseAccumulator
	if err := accumulator.Absorb(domain.NewToolCallEvent(domain.ToolCall{ID: "call_1", Name: "read", Arguments: arguments})); err != nil {
		t.Fatal(err)
	}
	if err := accumulator.Absorb(domain.NewFinishEvent(domain.FinishStop, nil)); err != nil {
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
	if got := domain.NewTextEndEvent(); got.Kind != domain.EventTextEnd {
		t.Fatalf("NewTextEndEvent() = %+v", got)
	}
	start := domain.NewToolCallStartEvent("call-1", "read")
	if start.Kind != domain.EventToolCallStart || start.ToolCallID != "call-1" || start.ToolName != "read" {
		t.Fatalf("NewToolCallStartEvent() = %+v", start)
	}
	delta := domain.NewToolCallDeltaEvent("call-1", `{"path"`)
	if delta.Kind != domain.EventToolCallDelta || delta.ToolCallID != "call-1" || delta.ArgumentsDelta != `{"path"` {
		t.Fatalf("NewToolCallDeltaEvent() = %+v", delta)
	}
	end := domain.NewToolCallEndEvent("call-1")
	if end.Kind != domain.EventToolCallEnd || end.ToolCallID != "call-1" {
		t.Fatalf("NewToolCallEndEvent() = %+v", end)
	}
}

func TestCollectRejectsNilStream(t *testing.T) {
	if _, err := usecase.Collect(context.Background(), nil); !errors.Is(err, domain.ErrInvalidRequest) {
		t.Fatalf("Collect(nil) error = %v, want ErrInvalidRequest", err)
	}
}

func TestCollectRejectsInvalidEventInStream(t *testing.T) {
	stream := &eventStream{events: []domain.Event{{Kind: "bad_kind"}}}
	if _, err := usecase.Collect(context.Background(), stream); err == nil {
		t.Fatal("expected error on stream with invalid event")
	}
	if stream.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", stream.closeCalls)
	}
}

func TestResponseAccumulatorNilSafety(t *testing.T) {
	var accumulator *usecase.ResponseAccumulator
	if err := accumulator.Absorb(domain.NewTextDeltaEvent("text")); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("nil Absorb() error = %v, want ErrInvalidEvent", err)
	}
	if _, err := accumulator.Finish(); !errors.Is(err, domain.ErrIncompleteStream) {
		t.Fatalf("nil Finish() error = %v, want ErrIncompleteStream", err)
	}
}

func TestResponseAccumulatorRejectsInvalidEvent(t *testing.T) {
	var accumulator usecase.ResponseAccumulator
	if err := accumulator.Absorb(domain.Event{Kind: "invalid"}); err == nil {
		t.Fatal("expected error on invalid event")
	}
}
