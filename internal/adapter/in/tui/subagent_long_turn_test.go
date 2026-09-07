package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/engine/turn"
	"github.com/projectTHORN/proton/internal/feature/agent"
)

func TestLongTurnWithSubagentsKeepsProgressCoherent(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(110, 30)
	m.busy = true
	m.busyStarted = time.Now().Add(-12 * time.Second)
	m.agentSnapshot = []agent.AgentStatus{
		{ID: "explorer-1", Task: "inspect router", State: agent.StateRunning, StartedAt: time.Now().Add(-10 * time.Second)},
		{ID: "reviewer-2", Task: "review safety", State: agent.StateRunning, StartedAt: time.Now().Add(-9 * time.Second)},
		{ID: "int-3", Task: "analyze boundaries", State: agent.StateQueued, StartTime: time.Now().Add(-8 * time.Second)},
	}
	m.agentActivity["explorer-1"] = "using grep"

	delegate, _ := tool.NewCall("d1", "delegate_task", json.RawMessage(`{"profile":"int","task":"inspect router"}`))
	wait, _ := tool.NewCall("w1", "wait_agent", json.RawMessage(`{"agent_id":"explorer-1"}`))
	m.applyTurnEvents([]turn.Event{
		{Kind: turn.EventToolCall, Round: 1, Call: delegate},
		{Kind: turn.EventToolResult, Round: 1, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "delegate_task", Output: `{"agent_id":"explorer-1","status":"queued"}`}},
		{Kind: turn.EventToolCall, Round: 2, Call: wait},
	})

	status := m.statusView()
	for _, want := range []string{"coordinating", "round 2", "2 tools", "3 agents"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status=%q, want %q", status, want)
		}
	}
	if panel := m.agentsView(); !strings.Contains(panel, "inspect router") || !strings.Contains(panel, "using grep") {
		t.Fatalf("busy agent panel lost active work: %q", panel)
	}
	if active := m.historyState.Active(); active != nil {
		t.Fatalf("wait_agent leaked an active orchestration cell=%T %#v", active, active)
	}
	if run := m.historyState.AgentRun("explorer-1"); run == nil || run.Task != "inspect router" {
		t.Fatalf("delegated run was not retained as one lifecycle cell: %#v", run)
	}

	m.busy = false
	panel := m.agentsView()
	if !strings.Contains(panel, "using grep") {
		t.Fatalf("idle agent panel lost live activity: %q", panel)
	}
}
