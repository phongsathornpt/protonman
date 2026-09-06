package protonsdk

import (
	"encoding/json"
	"testing"
)

func TestRequestRejectsNegativeMaxOutputTokens(t *testing.T) {
	req := Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}, Options: ModelOptions{MaxOutputTokens: -1}}
	if err := req.Validate(); err == nil {
		t.Fatal("expected validation error")
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
