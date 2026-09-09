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
