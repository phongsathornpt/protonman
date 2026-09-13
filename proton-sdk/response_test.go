package protonsdk

import (
	"context"
	"testing"
)

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
