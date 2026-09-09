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

func TestCollectStep(t *testing.T) {
	stream := &eventStream{events: []Event{
		{Kind: EventTextStart},
		{Kind: EventTextDelta, Text: "hello "},
		{Kind: EventTextDelta, Text: "world"},
		{Kind: EventToolCall, ToolCall: ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
		{Kind: EventFinish, FinishReason: FinishStop},
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
	stream := &eventStream{events: []Event{{Kind: EventTextDelta, Text: "partial"}}}
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
	stream := &eventStream{
		events:   []Event{{Kind: EventFinish, FinishReason: FinishStop}},
		closeErr: closeErr,
	}
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
