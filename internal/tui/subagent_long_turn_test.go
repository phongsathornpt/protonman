package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/turn"
)

func TestLongTurnWithSubagentsKeepsProgressCoherent(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(110, 30)
	m.busy = true
	m.busyStarted = time.Now().Add(-12 * time.Second)
	m.maxRounds = 10
	m.agentSnapshot = []agent.AgentStatus{
		{ID: "explorer-1", Task: "inspect router", State: agent.StateRunning, StartedAt: time.Now().Add(-10 * time.Second)},
		{ID: "reviewer-2", Task: "review safety", State: agent.StateRunning, StartedAt: time.Now().Add(-9 * time.Second)},
		{ID: "int-3", Task: "analyze boundaries", State: agent.StateQueued, StartTime: time.Now().Add(-8 * time.Second)},
	}
	m.agentActivity["explorer-1"] = "using grep"

	delegate, _ := tool.NewCall("d1", "delegate_task", json.RawMessage(`{"profile":"explorer","task":"inspect router"}`))
	wait, _ := tool.NewCall("w1", "wait_agent", json.RawMessage(`{"agent_id":"explorer-1"}`))
	m.applyTurnEvents([]turn.Event{
		{Kind: turn.EventToolCall, Round: 1, Call: delegate},
		{Kind: turn.EventToolResult, Round: 1, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "delegate_task", Output: `{"agent_id":"explorer-1","status":"queued"}`}},
		{Kind: turn.EventToolCall, Round: 2, Call: wait},
	})

	status := m.statusView()
	for _, want := range []string{"coordinating", "round 2/10", "2 tools", "3 agents"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status=%q, want %q", status, want)
		}
	}
	if panel := m.agentsView(); strings.Contains(panel, "\n") {
		t.Fatalf("busy agent panel should stay collapsed: %q", panel)
	}
	if active, ok := m.historyState.Active().(*AgentToolCell); !ok || active.Name != "wait_agent" {
		t.Fatalf("active orchestration cell=%T %#v", m.historyState.Active(), m.historyState.Active())
	}

	m.busy = false
	panel := m.agentsView()
	if !strings.Contains(panel, "using grep") {
		t.Fatalf("idle agent panel lost live activity: %q", panel)
	}
}
