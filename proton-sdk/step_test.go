package protonsdk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type eventStream struct {
	events []Event
	index  int
}

func (s *eventStream) Next(context.Context) (Event, error) {
	if s.index >= len(s.events) {
		return Event{}, io.EOF
	}
	e := s.events[s.index]
	s.index++
	return e, nil
}
func (*eventStream) Close() error { return nil }

func TestCollectStep(t *testing.T) {
	stream := &eventStream{events: []Event{
		{Kind: EventTextStart},
		{Kind: EventTextDelta, Text: "hello "},
		{Kind: EventTextDelta, Text: "world"},
		{Kind: EventToolCall, ToolCall: ToolCall{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
		{Kind: EventDone},
	}}
	result, err := CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatalf("CollectStep() error = %v", err)
	}
	if result.Text != "hello world" {
		t.Fatalf("Text = %q", result.Text)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "read_file" {
		t.Fatalf("ToolCalls = %#v", result.ToolCalls)
	}
}

func TestCollectStepRejectsIncompleteStream(t *testing.T) {
	_, err := CollectStep(context.Background(), &eventStream{events: []Event{{Kind: EventTextDelta, Text: "partial"}}})
	if !errors.Is(err, ErrIncompleteStream) {
		t.Fatalf("error = %v, want ErrIncompleteStream", err)
	}
}

func TestCollectStepIncludesUsageAndFinishReason(t *testing.T) {
	stream := &eventStream{events: []Event{
		{Kind: EventTextDelta, Text: "done"},
		{Kind: EventUsage, Usage: Usage{InputTokens: 4, OutputTokens: 1, TotalTokens: 5}},
		{Kind: EventFinish, FinishReason: FinishStop},
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
