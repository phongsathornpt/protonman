package agentui

import (
	"errors"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestApplyToolFailureClearsPendingSpawnIntent(t *testing.T) {
	tracker := Tracker{
		pendingRuns:    map[string]PendingRun{"call-1": {Task: "inspect"}},
		pendingActions: map[string]string{"call-1": "spawn"},
	}
	state := history.NewHistoryState(100)
	result := tool.Result{CallID: "call-1", ToolName: "subagent"}
	if !tracker.ApplyToolFailure("subagent", result, errors.New("boom"), state) {
		t.Fatal("expected subagent failure to be handled")
	}
	if _, ok := tracker.pendingRuns["call-1"]; ok {
		t.Fatal("pending spawn intent retained after failed call")
	}
	if _, ok := tracker.pendingActions["call-1"]; ok {
		t.Fatal("pending action retained after failed call")
	}
}

func TestApplyToolResultPreservesActivityForRunningAgent(t *testing.T) {
	tracker := Tracker{
		pendingActions: map[string]string{"call-2": "get"},
	}
	state := history.NewHistoryState(100)
	result := tool.Result{CallID: "call-2", ToolName: "subagent"}
	body := `{"action":"get","agent_id":"agent-1","profile":"strength","status":"running"}`
	if !tracker.ApplyToolResult("subagent", result, body, state) {
		t.Fatal("expected subagent result to be applied")
	}
	cell := state.AgentRun("agent-1")
	if cell == nil {
		t.Fatal("expected agent-1 run cell in state")
	}
	if cell.State != "running" {
		t.Fatalf("expected state running, got %s", cell.State)
	}
	if cell.Activity == "" {
		t.Fatal("expected non-empty activity for running agent")
	}
}

func TestApplyToolResultClearsActivityForTerminalAgent(t *testing.T) {
	tracker := Tracker{
		pendingActions: map[string]string{"call-3": "get"},
	}
	state := history.NewHistoryState(100)
	result := tool.Result{CallID: "call-3", ToolName: "subagent"}
	body := `{"action":"get","agent_id":"agent-2","profile":"strength","status":"completed"}`
	if !tracker.ApplyToolResult("subagent", result, body, state) {
		t.Fatal("expected subagent result to be applied")
	}
	cell := state.AgentRun("agent-2")
	if cell == nil {
		t.Fatal("expected agent-2 run cell in state")
	}
	if cell.State != "completed" {
		t.Fatalf("expected state completed, got %s", cell.State)
	}
	if cell.Activity != "" {
		t.Fatalf("expected empty activity for completed agent, got %q", cell.Activity)
	}
	if cell.FinishedAt.IsZero() {
		t.Fatal("expected FinishedAt to be set for completed agent")
	}
}
