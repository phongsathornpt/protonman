package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/engine/turn"
)

func TestSubagentLifecycleCollapsesIntoOneRunCell(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	delegate, _ := tool.NewCall("d1", "delegate_task", json.RawMessage(`{"profile":"int","task":"inspect router"}`))
	wait, _ := tool.NewCall("w1", "wait_agent", json.RawMessage(`{"agent_id":"int-7"}`))

	m.applyTurnEvents([]turn.Event{
		{Kind: turn.EventToolCall, Call: delegate},
		{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "delegate_task", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"queued"}`)}},
		{Kind: turn.EventToolCall, Call: wait},
		{Kind: turn.EventToolResult, Call: wait, Result: tool.Result{CallID: "w1", ToolName: "wait_agent", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"completed","result":{"summary":"found routing issue"}}`)}},
	})

	cells := m.historyState.Cells()
	if len(cells) != 1 {
		t.Fatalf("history cells=%d, want one delegated run: %#v", len(cells), cells)
	}
	run, ok := cells[0].(*AgentRunCell)
	if !ok || run.AgentID != "int-7" || run.Task != "inspect router" || run.Summary != "found routing issue" {
		t.Fatalf("run=%T %#v", cells[0], cells[0])
	}
	plain := strings.Join(run.RawLines(), "\n")
	for _, leaked := range []string{"wait_agent", "Waiting for", "Checking subagents"} {
		if strings.Contains(plain, leaked) {
			t.Fatalf("orchestration detail leaked into run cell: %q", plain)
		}
	}
}

func TestSubagentRunsStayDistinctByAgentID(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	for _, tc := range []struct{ callID, agentID, task string }{
		{"d1", "int-1", "inspect router"},
		{"d2", "int-2", "inspect cache"},
	} {
		call, _ := tool.NewCall(tc.callID, "delegate_task", json.RawMessage(`{"profile":"int","task":"`+tc.task+`"}`))
		m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: call})
		m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: call, Result: tool.Result{CallID: tc.callID, ToolName: "delegate_task", StructuredOutput: json.RawMessage(`{"agent_id":"` + tc.agentID + `","status":"queued"}`)}})
	}
	if m.historyState.AgentRun("int-1") == nil || m.historyState.AgentRun("int-2") == nil {
		t.Fatalf("parallel runs were conflated: %#v", m.historyState.Cells())
	}
	if len(m.historyState.Cells()) != 2 {
		t.Fatalf("cells=%d, want 2", len(m.historyState.Cells()))
	}
}
