package turn

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestToolResultBudgetTruncatesPayloadAndPreservesMetadata(t *testing.T) {
	budget := newToolResultBudget(64, 64)
	failure := &tool.Failure{Code: tool.ErrorCodeExecution, Message: "failed"}
	executions := []executedCall{{result: tool.Result{
		Output:           strings.Repeat("ก", 40),
		Stdout:           strings.Repeat("x", 30),
		Stderr:           "stderr",
		StructuredOutput: json.RawMessage(`{"large":"payload"}`),
		StdoutBytes:      30,
		StderrBytes:      6,
		Failure:          failure,
		CheckpointID:     "cp-1",
		AffectedPaths:    []string{"a.go"},
	}}}
	got := budget.applyRound(executions)[0].result
	if !got.Truncated || !strings.Contains(got.Output, toolBudgetMarker) {
		t.Fatalf("truncated result = %+v", got)
	}
	if !utf8.ValidString(got.Output) {
		t.Fatalf("truncated output is invalid UTF-8: %q", got.Output)
	}
	if got.Stdout != "" || got.Stderr != "" {
		t.Fatalf("duplicate stream fields survived truncation: %+v", got)
	}
	if gotStructured := string(got.StructuredOutput); gotStructured != `{"large":"payload"}` {
		t.Fatalf("structured output = %q, want preserved valid JSON", gotStructured)
	}
	if got.Failure != failure || got.CheckpointID != "cp-1" || len(got.AffectedPaths) != 1 {
		t.Fatalf("metadata was not preserved: %+v", got)
	}
	if !got.StdoutTruncated || !got.StderrTruncated {
		t.Fatalf("stream truncation flags = stdout:%v stderr:%v", got.StdoutTruncated, got.StderrTruncated)
	}
}

func TestToolResultBudgetTracksTurnAcrossRounds(t *testing.T) {
	budget := newToolResultBudget(0, 20)
	first := budget.applyRound([]executedCall{{result: tool.Result{Output: strings.Repeat("a", 15)}}})
	if first[0].result.Truncated {
		t.Fatal("first result unexpectedly truncated")
	}
	second := budget.applyRound([]executedCall{{result: tool.Result{Output: strings.Repeat("b", 15)}}})
	if !second[0].result.Truncated {
		t.Fatalf("second result = %+v, want turn-budget truncation", second[0].result)
	}
	if got := toolResultTextBytes(second[0].result); got > 5 {
		t.Fatalf("second result text bytes = %d, want <= remaining 5", got)
	}
	if budget.turnUsed != 20 {
		t.Fatalf("turnUsed = %d, want 20", budget.turnUsed)
	}
}

func TestToolResultBudgetTracksRoundSeparately(t *testing.T) {
	budget := newToolResultBudget(25, 0)
	got := budget.applyRound([]executedCall{
		{result: tool.Result{Output: strings.Repeat("a", 20)}},
		{result: tool.Result{Output: strings.Repeat("b", 20)}},
	})
	if got[0].result.Truncated {
		t.Fatal("first result unexpectedly truncated")
	}
	if !got[1].result.Truncated {
		t.Fatal("second result was not truncated by round budget")
	}
}

func TestToolResultBudgetPreservesStructuredOutputBeforeHumanText(t *testing.T) {
	structured := json.RawMessage(`{"items":[1,2,3]}`)
	allowed := len(structured) + 16
	budget := newToolResultBudget(allowed, allowed)
	got := budget.applyRound([]executedCall{{result: tool.Result{
		Output:           strings.Repeat("human summary ", 20),
		StructuredOutput: structured,
	}}})[0].result
	if string(got.StructuredOutput) != string(structured) {
		t.Fatalf("structured output = %q, want %q", got.StructuredOutput, structured)
	}
	if !got.Truncated {
		t.Fatal("result was not marked truncated")
	}
	if toolResultTextBytes(got) > allowed {
		t.Fatalf("result text bytes = %d, want <= %d", toolResultTextBytes(got), allowed)
	}
}

func TestToolResultBudgetTurnsOversizedStructuredOutputIntoTypedFailure(t *testing.T) {
	budget := newToolResultBudget(16, 16)
	got := budget.applyRound([]executedCall{{result: tool.Result{
		StructuredOutput: json.RawMessage(`{"items":["this payload cannot fit"]}`),
	}}})[0].result
	if len(got.StructuredOutput) != 0 {
		t.Fatalf("structured output survived impossible budget: %q", got.StructuredOutput)
	}
	if got.Failure == nil || got.Failure.Code != tool.ErrorCodeOutputTooLarge {
		t.Fatalf("failure = %#v, want output_too_large", got.Failure)
	}
	if !got.Truncated {
		t.Fatal("result was not marked truncated")
	}
	if toolResultTextBytes(got) > 16 {
		t.Fatalf("result text bytes = %d, want <= 16", toolResultTextBytes(got))
	}
}

func TestTruncateUTF8WithMarkerRespectsTinyLimit(t *testing.T) {
	for _, limit := range []int{0, 1, 8, len(toolBudgetMarker)} {
		got := truncateUTF8WithMarker("payload", limit, toolBudgetMarker)
		if len(got) > limit {
			t.Fatalf("limit %d produced %d bytes: %q", limit, len(got), got)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("limit %d produced invalid UTF-8: %q", limit, got)
		}
	}
}
