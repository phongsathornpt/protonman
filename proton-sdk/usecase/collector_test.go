package usecase_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

type testStream struct {
	events []domain.Event
	idx    int
	closed bool
}

func (s *testStream) Next(ctx context.Context) (domain.Event, error) {
	if s.idx >= len(s.events) {
		return domain.Event{}, io.EOF
	}
	ev := s.events[s.idx]
	s.idx++
	return ev, nil
}

func (s *testStream) Close() error {
	s.closed = true
	return nil
}

func TestCollect(t *testing.T) {
	stream := &testStream{
		events: []domain.Event{
			domain.NewTextStartEvent(),
			domain.NewTextDeltaEvent("Hello "),
			domain.NewTextDeltaEvent("World"),
			domain.NewReasoningDeltaEvent("inspect first"),
			domain.NewToolCallEvent(domain.ToolCall{ID: "c1", Name: "echo", Arguments: json.RawMessage(`{}`)}),
			domain.NewUsageEvent(domain.Usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15}),
			domain.NewFinishEvent(domain.FinishStop, domain.ProviderMetadata{"prov": json.RawMessage(`{}`)}),
		},
	}
	resp, err := usecase.Collect(context.Background(), stream)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if resp.Text != "Hello World" {
		t.Fatalf("unexpected text: %q", resp.Text)
	}
	if resp.ReasoningContent != "inspect first" {
		t.Fatalf("unexpected reasoning content: %q", resp.ReasoningContent)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "echo" {
		t.Fatalf("unexpected tool calls: %+v", resp.ToolCalls)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
	if !stream.closed {
		t.Fatal("stream should be closed")
	}

	// Test CollectStep
	stream2 := &testStream{
		events: []domain.Event{
			domain.NewTextDeltaEvent("step"),
			domain.NewFinishEvent(domain.FinishStop, nil),
		},
	}
	step, err := usecase.CollectStep(context.Background(), stream2)
	if err != nil || step.Text != "step" {
		t.Fatalf("CollectStep failed: %v", err)
	}
}

func TestResponseAccumulatorLifecycle(t *testing.T) {
	var acc usecase.ResponseAccumulator
	if _, err := acc.Finish(); err == nil {
		t.Fatal("finishing unstarted accumulator should fail")
	}
	if err := acc.Absorb(domain.NewTextDeltaEvent("hi")); err != nil {
		t.Fatal(err)
	}
	if err := acc.Absorb(domain.NewFinishEvent(domain.FinishStop, nil)); err != nil {
		t.Fatal(err)
	}
	resp, err := acc.Finish()
	if err != nil || resp.Text != "hi" {
		t.Fatalf("unexpected finish result: %v, text=%q", err, resp.Text)
	}
	if err := acc.Absorb(domain.NewTextDeltaEvent("more")); err == nil {
		t.Fatal("absorbing after finish should fail")
	}
}
