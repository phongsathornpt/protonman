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

func TestAgentPollingFailureDoesNotLeakRPCTranscript(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	delegate, _ := tool.NewCall("d1", "delegate_task", json.RawMessage(`{"profile":"int","task":"inspect router"}`))
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: delegate})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "delegate_task", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"running"}`)}})

	wait, _ := tool.NewCall("w1", "wait_agent", json.RawMessage(`{"agent_id":"int-7"}`))
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: wait})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: wait, Result: tool.Result{
		CallID: "w1", ToolName: "wait_agent",
		Failure: &tool.Failure{Code: tool.ErrorCodeDeadlineExceeded, Message: "wait timeout"},
	}})

	plain := m.historyState.Raw()
	for _, leaked := range []string{"Wait agent", "wait_agent", "Waiting for int-7", "deadline_exceeded"} {
		if strings.Contains(plain, leaked) {
			t.Fatalf("orchestration failure leaked into transcript: %q", plain)
		}
	}
	run := m.historyState.AgentRun("int-7")
	if run == nil || !strings.Contains(run.Activity, "status check failed") {
		t.Fatalf("run activity=%#v", run)
	}
}

func TestTerminalAgentLeavesLivePaneButStaysInTranscript(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	started := time.Now().Add(-5 * time.Second)
	run := &AgentRunCell{AgentID: "dex-9", Profile: agent.ProfileDEX, Task: "review concurrency", State: agent.StateRunning, StartedAt: started}
	m.historyState.Append(run)
	m.agentSnapshot = []agent.AgentStatus{{
		ID: "dex-9", Profile: agent.ProfileDEX, Task: "review concurrency",
		State: agent.StateFailed, StartedAt: started, FinishedAt: time.Now(), Reason: "timed out",
	}}
	m.syncAgentRunSnapshot("dex-9")
	if got := m.agentsView(); got != "" {
		t.Fatalf("terminal agent leaked into live pane: %q", got)
	}
	plain := m.historyState.Raw()
	if !strings.Contains(plain, "review concurrency") || !strings.Contains(plain, "timed out") {
		t.Fatalf("terminal run missing from transcript: %q", plain)
	}
}

func TestOutOfOrderAgentResultMergesIntoDelegateRun(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	delegate, _ := tool.NewCall("d1", "delegate_task", json.RawMessage(`{"profile":"int","task":"inspect router"}`))
	get, _ := tool.NewCall("g1", "get_agent", json.RawMessage(`{"agent_id":"int-7"}`))

	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: delegate})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: get})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: get, Result: tool.Result{
		CallID: "g1", ToolName: "get_agent",
		StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"running"}`),
	}})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{
		CallID: "d1", ToolName: "delegate_task",
		StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"running"}`),
	}})

	cells := m.historyState.Cells()
	if len(cells) != 1 {
		t.Fatalf("out-of-order lifecycle created duplicate cells: %#v", cells)
	}
	run, ok := cells[0].(*AgentRunCell)
	if !ok || run.AgentID != "int-7" || run.Profile != agent.ProfileINT || run.Task != "inspect router" {
		t.Fatalf("merged run=%T %#v", cells[0], cells[0])
	}
}

func TestCancelAgentUpdatesExistingRunWithoutExtraCell(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	delegate, _ := tool.NewCall("d1", "delegate_task", json.RawMessage(`{"profile":"dex","task":"review concurrency"}`))
	cancel, _ := tool.NewCall("c1", "cancel_agent", json.RawMessage(`{"agent_id":"dex-7"}`))
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: delegate})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{
		CallID: "d1", ToolName: "delegate_task",
		StructuredOutput: json.RawMessage(`{"agent_id":"dex-7","status":"running"}`),
	}})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: cancel})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: cancel, Result: tool.Result{
		CallID: "c1", ToolName: "cancel_agent",
		StructuredOutput: json.RawMessage(`{"agent_id":"dex-7","status":"canceled"}`),
	}})
	cells := m.historyState.Cells()
	if len(cells) != 1 {
		t.Fatalf("cancel created extra transcript cells: %#v", cells)
	}
	run := m.historyState.AgentRun("dex-7")
	if run == nil || run.State != agent.StateCanceled {
		t.Fatalf("canceled run=%#v", run)
	}
}

func TestDelegateMissingAgentIDFallsBackWithoutCorruptingHistory(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	delegate, _ := tool.NewCall("d-missing", "delegate_task", json.RawMessage(`{"profile":"int","task":"inspect router"}`))
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: delegate})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{
		CallID: "d-missing", ToolName: "delegate_task",
		StructuredOutput: json.RawMessage(`{"status":"queued"}`),
	}})
	cells := m.historyState.Cells()
	if len(cells) != 1 {
		t.Fatalf("missing agent id corrupted history: %#v", cells)
	}
	if run, ok := cells[0].(*AgentRunCell); ok && run.AgentID == "" {
		t.Fatalf("malformed response created unaddressable run cell: %#v", run)
	}
}
