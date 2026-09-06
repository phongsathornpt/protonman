package turn

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/projectTHORN/proton/internal/tool"
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
	if got.Stdout != "" || got.Stderr != "" || len(got.StructuredOutput) != 0 {
		t.Fatalf("duplicate payload fields survived truncation: %+v", got)
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
	if !second[0].result.Truncated || !strings.Contains(second[0].result.Output, toolBudgetMarker) {
		t.Fatalf("second result = %+v, want turn-budget truncation", second[0].result)
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
