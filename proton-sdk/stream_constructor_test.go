package protonsdk

import (
	"encoding/json"
	"testing"
)

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
