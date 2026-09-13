package protonsdk

import (
	"encoding/json"
	"errors"
	"testing"
)

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
