package runtime

import (
	"context"
	"encoding/json"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"strings"
	"testing"
	"time"
)

func TestAgentRuntimeStateSurvivesBubbleModelRestart(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	state := newAgentRuntimeState(config.AgentConfig{MaxToolCalls: 17, Profile: "dex", SubagentsEnabled: true, ReasoningEffort: sdk.ReasoningHigh}, true)
	first := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "")
	first.agents = app.NewAgents(coord)
	state.apply(first)
	first.maxToolCalls = 23
	first.agentProfile = "strength"
	first.subagentsEnabled = false
	first.reasoningEffort = sdk.ReasoningLow
	coord.SetEnabled(false)
	state.capture(first)
	restarted := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "")
	restarted.agents = app.NewAgents(coord)
	state.apply(restarted)
	if restarted.maxToolCalls != 23 || restarted.agentProfile != "strength" || restarted.subagentsEnabled || restarted.reasoningEffort != sdk.ReasoningLow {
		t.Fatalf("restart state = tool_calls=%d profile=%q subagents=%v reasoning=%q", restarted.maxToolCalls, restarted.agentProfile, restarted.subagentsEnabled, restarted.reasoningEffort)
	}
	if coord.Enabled() {
		t.Fatal("restart re-enabled coordinator")
	}
}

type blockingAgentViewRunner struct{ release <-chan struct{} }

func (r blockingAgentViewRunner) Run(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
	select {
	case <-r.release:
		return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
	case <-ctx.Done():
		return turn.Result{}, ctx.Err()
	}
}

func TestAgentsViewShowsActiveAndRespectsLayout(t *testing.T) {
	release := make(chan struct{})
	coord := agent.NewCoordinator(nil, nil, nil, nil, agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
		return blockingAgentViewRunner{release: release}, nil
	}))
	defer coord.Close()
	if _, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileAgility, Task: "inspect router"}); err != nil {
		t.Fatal(err)
	}
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.agents = app.NewAgents(coord)
	m.agentSnapshot = coord.List()
	m.resize(80, 24)
	if got := m.agentsView(); !strings.Contains(got, "Agents 1 active") || !strings.Contains(got, "inspect router") {
		t.Fatalf("agents view=%q", got)
	}
	m.resize(60, 18)
	if got := m.agentsView(); !strings.Contains(got, "Agents 1 active") || !strings.Contains(got, "inspect router") {
		t.Fatalf("compact agents view=%q", got)
	}
	m.resize(24, 12)
	if got := m.agentsView(); got != "" {
		t.Fatalf("tiny agents view=%q", got)
	}
	close(release)
}

func TestDisabledSubagentsAppearInFooterAndAgentsPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.subagentsEnabled = false
	if got := m.infoView(); !strings.Contains(got, "subagents off") {
		t.Fatalf("info view=%q, want disabled subagent indicator", got)
	}
	rows := agentInspectionRows(m)
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "Subagents disabled") || !strings.Contains(joined, "Universal handles work directly") {
		t.Fatalf("agents pane=%q", joined)
	}
}

func TestDisabledSubagentsKeepExistingAgentsManageableInPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.subagentsEnabled = false
	m.agentSnapshot = []agent.AgentStatus{{ID: "int-1", Profile: agent.ProfileAgility, Task: "inspect", State: agent.StateRunning, StartedAt: time.Now()}}
	joined := strings.Join(agentInspectionRows(m), "\n")
	if !strings.Contains(joined, "New delegation disabled") || !strings.Contains(joined, "AGI") {
		t.Fatalf("agents pane=%q", joined)
	}
}

func TestAgentLifecycleMessageRefreshesSnapshot(t *testing.T) {
	release := make(chan struct{})
	coord := agent.NewCoordinator(nil, nil, nil, nil, agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
		return blockingAgentViewRunner{release: release}, nil
	}))
	defer coord.Close()
	events, cancel := coord.Subscribe(8)
	defer cancel()
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.agents = app.NewAgents(coord)
	m.agentEvents = events
	if _, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileAgility, Task: "inspect router"}); err != nil {
		t.Fatal(err)
	}
	msg := (<-events)
	updated, _ := m.Update(agentLifecycleMsg{event: msg})
	m = updated.(*bubbleModel)
	if len(m.agentSnapshot) != 1 {
		t.Fatalf("agent snapshot=%#v", m.agentSnapshot)
	}
	close(release)
}

func TestAgentsViewPrioritizesActiveAndShowsCanceling(t *testing.T) {
	now := time.Now()
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.agentSnapshot = []agent.AgentStatus{{ID: "done-1", Task: "old result", State: agent.StateCompleted, StartedAt: now.Add(-20 * time.Second), FinishedAt: now.Add(-15 * time.Second)}, {ID: "done-2", Task: "new result", State: agent.StateCompleted, StartedAt: now.Add(-10 * time.Second), FinishedAt: now.Add(-9 * time.Second)}, {ID: "run-1", Profile: agent.ProfileAgility, Task: "inspect active", State: agent.StateRunning, StartedAt: now.Add(-3 * time.Second)}, {ID: "cancel-1", Profile: agent.ProfileIntelligence, Task: "stop active", State: agent.StateCanceling, StartedAt: now.Add(-4 * time.Second)}}
	m.resize(100, 30)
	got := m.agentsView()
	if !strings.Contains(got, "Agents 2 active") || !strings.Contains(got, "1 running") || !strings.Contains(got, "1 canceling") {
		t.Fatalf("agents view summary=%q", got)
	}
	if !strings.Contains(got, "AGI") || !strings.Contains(got, "INT") || strings.Contains(got, "run-1") || strings.Contains(got, "cancel-1") {
		t.Fatalf("agent identities were not normalized: %q", got)
	}
}

func TestAgentDisplayDurationUsesExecutionDurationForTerminalState(t *testing.T) {
	started := time.Unix(100, 0)
	finished := started.Add(7 * time.Second)
	st := agent.AgentStatus{State: agent.StateCompleted, StartedAt: started, FinishedAt: finished}
	if got := agentDisplayDuration(st, finished.Add(time.Hour)); got != 7*time.Second {
		t.Fatalf("terminal display duration=%v, want 7s", got)
	}
}

func TestStatusViewShowsSubagentCoordinationDuringBusyTurn(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(80, 24)
	m.busy = true
	m.busyStarted = time.Now().Add(-4 * time.Second)
	m.activity = "thinking"
	m.agentSnapshot = []agent.AgentStatus{{ID: "explorer-1", State: agent.StateRunning}, {ID: "reviewer-2", State: agent.StateQueued}}
	got := m.statusView()
	if !strings.Contains(got, "coordinating") || !strings.Contains(got, "2 agents") {
		t.Fatalf("status view=%q", got)
	}
}

func TestStatusViewKeepsAgentCoordinationVisibleInTinyLayout(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(24, 12)
	m.busy = true
	m.busyStarted = time.Now()
	m.agentSnapshot = []agent.AgentStatus{{ID: "explorer-1", State: agent.StateRunning}}
	if got := m.agentsView(); got != "" {
		t.Fatalf("tiny agents view=%q, want hidden panel", got)
	}
	if got := m.statusView(); !strings.Contains(got, "coordinating") {
		t.Fatalf("tiny status view=%q, want coordination state", got)
	}
}

func TestAgentLifecycleProgressShowsCurrentActivity(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.agentSnapshot = []agent.AgentStatus{{ID: "explorer-1", Task: "inspect router", State: agent.StateRunning, StartedAt: time.Now()}}
	m.resize(100, 30)
	call, _ := tool.NewCall("grep-1", "grep", []byte(`{"pattern":"routeRequest","path":"internal"}`))
	updated, _ := m.Update(agentLifecycleMsg{event: agent.Event{Kind: agent.EventAgentProgress, AgentID: "explorer-1", Call: &call}})
	m = updated.(*bubbleModel)
	if len(m.agentSnapshot) != 1 {
		t.Fatalf("progress event unexpectedly replaced agent snapshot: %#v", m.agentSnapshot)
	}
	got := m.agentsView()
	if !strings.Contains(got, "routeRequest") || strings.Contains(got, "using grep") {
		t.Fatalf("agents view=%q, want structured tool activity", got)
	}
	updated, _ = m.Update(agentLifecycleMsg{event: agent.Event{Kind: agent.EventAgentCompleted, AgentID: "explorer-1"}})
	m = updated.(*bubbleModel)
	if _, ok := m.agentActivity["explorer-1"]; ok {
		t.Fatal("terminal lifecycle event did not clear transient activity")
	}
}

func TestAgentsViewShowsActiveWorkDuringBusyRootTurn(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.busy = true
	m.agentSnapshot = []agent.AgentStatus{{ID: "explorer-1", Task: "inspect router", State: agent.StateRunning, StartedAt: time.Now()}, {ID: "reviewer-2", Task: "review risks", State: agent.StateQueued}}
	got := m.agentsView()
	if !strings.Contains(got, "Agents 2 active") {
		t.Fatalf("busy agents view=%q", got)
	}
	if !strings.Contains(got, "inspect router") || !strings.Contains(got, "review risks") {
		t.Fatalf("busy agents view hid delegated work: %q", got)
	}
}

func TestStatusViewCombinesRootAndSubagentProgress(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(120, 30)
	m.busy = true
	m.busyStarted = time.Now().Add(-8 * time.Second)
	m.turnProgress = turnProgress{Round: 3, ToolCalls: 8}
	m.agentSnapshot = []agent.AgentStatus{{ID: "explorer-1", State: agent.StateRunning}}
	got := m.statusView()
	for _, want := range []string{"coordinating", "1 agent"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status view=%q, want %q", got, want)
		}
	}
}

func TestApplyTurnEventTracksRoundAndToolCount(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Round: 2, Call: tool.Call{ID: "c1", Name: "read_file"}})
	if m.turnProgress.Round != 2 || m.turnProgress.ToolCalls != 1 {
		t.Fatalf("turn progress=%+v, want round 2 and 1 tool", m.turnProgress)
	}
}

func TestAgentsViewHidesTerminalAgents(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.agentSnapshot = []agent.AgentStatus{{ID: "reviewer-2", Task: "review security", State: agent.StateFailed, StartedAt: time.Now().Add(-10 * time.Second), FinishedAt: time.Now(), Reason: "timed out"}}
	if got := m.agentsView(); got != "" {
		t.Fatalf("terminal agent leaked into live pane: %q", got)
	}
}

func TestStatusViewAvoidsDuplicatingAgentPaneDetail(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(140, 30)
	m.busy = true
	m.agentSnapshot = []agent.AgentStatus{{ID: "explorer-1", State: agent.StateRunning}, {ID: "reviewer-2", State: agent.StateQueued}}
	got := m.statusView()
	if !strings.Contains(got, "coordinating 2 agents") {
		t.Fatalf("status=%q", got)
	}
	for _, duplicate := range []string{"1 running", "1 queued", "using grep", "round", "tools"} {
		if strings.Contains(got, duplicate) {
			t.Fatalf("status duplicated pane detail %q: %q", duplicate, got)
		}
	}
}

func TestCancelActiveTurnCancelsOnlyOwnedSubagents(t *testing.T) {
	release := make(chan struct{})
	coord := agent.NewCoordinator(nil, nil, nil, nil, agent.WithMaxConcurrency(2), agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
		return blockingAgentViewRunner{release: release}, nil
	}))
	defer func() {
		close(release)
		_ = coord.Close()
	}()
	owned, err := coord.Spawn(context.Background(), agent.Request{ParentID: "turn-owned", Profile: agent.ProfileAgility, Task: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := coord.Spawn(context.Background(), agent.Request{ParentID: "turn-other", Profile: agent.ProfileAgility, Task: "other"})
	if err != nil {
		t.Fatal(err)
	}
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.agents = app.NewAgents(coord)
	m.agentSnapshot = coord.List()
	m.activeTurnOwner = "turn-owned"
	m.busy = true
	m.busyStarted = time.Now()
	cancelled := false
	m.turnCancel = func() {
		cancelled = true
	}
	if got := m.cancelActiveTurn(); got != 1 {
		t.Fatalf("cancelActiveTurn()=%d, want 1", got)
	}
	if !cancelled {
		t.Fatal("root turn cancel was not invoked")
	}
	if m.activity != "canceling" {
		t.Fatalf("activity=%q, want canceling", m.activity)
	}
	ownedStatus, _ := coord.Get(owned.ID)
	if ownedStatus.State != agent.StateCanceling && ownedStatus.State != agent.StateCanceled {
		t.Fatalf("owned state=%s", ownedStatus.State)
	}
	otherStatus, _ := coord.Get(other.ID)
	if otherStatus.State == agent.StateCanceling || otherStatus.State == agent.StateCanceled {
		t.Fatalf("other state=%s, want unaffected", otherStatus.State)
	}
	if got := m.statusView(); !strings.Contains(got, "canceling") || !strings.Contains(got, "stopping 1 agents") {
		t.Fatalf("status=%q", got)
	}
}

func TestBusyAgentPanelScopesToActiveTurnOwner(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.busy = true
	m.activeTurnOwner = "turn-current"
	m.agentSnapshot = []agent.AgentStatus{{ID: "current", ParentID: "turn-current", Task: "current task", State: agent.StateRunning}, {ID: "old", ParentID: "turn-old", Task: "old task", State: agent.StateRunning}}
	got := m.agentsView()
	if !strings.Contains(got, "Agents 1 active") || strings.Contains(got, "old task") {
		t.Fatalf("agents view=%q", got)
	}
}

func TestAgentsCommandOpensFocusedInspectionPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.agentSnapshot = []agent.AgentStatus{{ID: "int-7", Profile: agent.ProfileAgility, Task: "inspect router", State: agent.StateRunning, StartedAt: time.Now().Add(-4 * time.Second)}, {ID: "dex-8", Profile: agent.ProfileIntelligence, Task: "review concurrency", State: agent.StateFailed, StartedAt: time.Now().Add(-6 * time.Second), FinishedAt: time.Now(), Reason: "timed out"}}
	m.agentActivity["int-7"] = AgentActivity{Label: `Search "routeRequest"`}
	_ = m.executeCommand("/agents")
	pane := m.bottom.find(agentsViewID)
	if pane == nil {
		t.Fatal("/agents did not open inspection pane")
	}
	got := pane.Render(m)
	for _, want := range []string{"AGI", "inspect router", "int-7", "INT", "review concurrency", "dex-8", "timed out"} {
		if !strings.Contains(got, want) {
			t.Fatalf("agents pane=%q, want %q", got, want)
		}
	}
}

func TestAgentsPaneShowsBoundModelIdentity(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.agentSnapshot = []agent.AgentStatus{{ID: "agility-1", Profile: agent.ProfileAgility, Provider: "openai", Model: "fast-model", Task: "inspect router", State: agent.StateRunning, StartedAt: time.Now()}}
	joined := strings.Join(agentInspectionRows(m), "\n")
	if !strings.Contains(joined, "openai · fast-model") {
		t.Fatalf("agents pane=%q, want bound model identity", joined)
	}
}

func TestSubagentLifecycleCollapsesIntoOneRunCell(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	delegate, _ := tool.NewCall("d1", "delegate_task", json.RawMessage(`{"profile":"int","task":"inspect router"}`))
	wait, _ := tool.NewCall("w1", "wait_agent", json.RawMessage(`{"agent_id":"int-7"}`))
	m.applyTurnEvents([]turn.Event{{Kind: turn.EventToolCall, Call: delegate}, {Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "delegate_task", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"queued"}`)}}, {Kind: turn.EventToolCall, Call: wait}, {Kind: turn.EventToolResult, Call: wait, Result: tool.Result{CallID: "w1", ToolName: "wait_agent", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"completed","result":{"summary":"found routing issue"}}`)}}})
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
	for _, tc := range []struct{ callID, agentID, task string }{{"d1", "int-1", "inspect router"}, {"d2", "int-2", "inspect cache"}} {
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
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: wait, Result: tool.Result{CallID: "w1", ToolName: "wait_agent", Failure: &tool.Failure{Code: tool.ErrorCodeDeadlineExceeded, Message: "wait timeout"}}})
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
	run := &AgentRunCell{AgentID: "dex-9", Profile: agent.ProfileIntelligence, Task: "review concurrency", State: agent.StateRunning, StartedAt: started}
	m.historyState.Append(run)
	m.agentSnapshot = []agent.AgentStatus{{ID: "dex-9", Profile: agent.ProfileIntelligence, Task: "review concurrency", State: agent.StateFailed, StartedAt: started, FinishedAt: time.Now(), Reason: "timed out"}}
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
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: get, Result: tool.Result{CallID: "g1", ToolName: "get_agent", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"running"}`)}})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "delegate_task", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"running"}`)}})
	cells := m.historyState.Cells()
	if len(cells) != 1 {
		t.Fatalf("out-of-order lifecycle created duplicate cells: %#v", cells)
	}
	run, ok := cells[0].(*AgentRunCell)
	if !ok || run.AgentID != "int-7" || run.Profile != agent.ProfileAgility || run.Task != "inspect router" {
		t.Fatalf("merged run=%T %#v", cells[0], cells[0])
	}
}

func TestCancelAgentUpdatesExistingRunWithoutExtraCell(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	delegate, _ := tool.NewCall("d1", "delegate_task", json.RawMessage(`{"profile":"dex","task":"review concurrency"}`))
	cancel, _ := tool.NewCall("c1", "cancel_agent", json.RawMessage(`{"agent_id":"dex-7"}`))
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: delegate})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "delegate_task", StructuredOutput: json.RawMessage(`{"agent_id":"dex-7","status":"running"}`)}})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: cancel})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: cancel, Result: tool.Result{CallID: "c1", ToolName: "cancel_agent", StructuredOutput: json.RawMessage(`{"agent_id":"dex-7","status":"canceled"}`)}})
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
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d-missing", ToolName: "delegate_task", StructuredOutput: json.RawMessage(`{"status":"queued"}`)}})
	cells := m.historyState.Cells()
	if len(cells) != 1 {
		t.Fatalf("missing agent id corrupted history: %#v", cells)
	}
	if run, ok := cells[0].(*AgentRunCell); ok && run.AgentID == "" {
		t.Fatalf("malformed response created unaddressable run cell: %#v", run)
	}
}

func TestLongTurnWithSubagentsKeepsProgressCoherent(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(110, 30)
	m.busy = true
	m.busyStarted = time.Now().Add(-12 * time.Second)
	m.agentSnapshot = []agent.AgentStatus{{ID: "explorer-1", Task: "inspect router", State: agent.StateRunning, StartedAt: time.Now().Add(-10 * time.Second)}, {ID: "reviewer-2", Task: "review safety", State: agent.StateRunning, StartedAt: time.Now().Add(-9 * time.Second)}, {ID: "int-3", Task: "analyze boundaries", State: agent.StateQueued, StartTime: time.Now().Add(-8 * time.Second)}}
	m.agentActivity["explorer-1"] = AgentActivity{Label: "using grep"}
	delegate, _ := tool.NewCall("d1", "delegate_task", json.RawMessage(`{"profile":"int","task":"inspect router"}`))
	wait, _ := tool.NewCall("w1", "wait_agent", json.RawMessage(`{"agent_id":"explorer-1"}`))
	m.applyTurnEvents([]turn.Event{{Kind: turn.EventToolCall, Round: 1, Call: delegate}, {Kind: turn.EventToolResult, Round: 1, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "delegate_task", Output: `{"agent_id":"explorer-1","status":"queued"}`}}, {Kind: turn.EventToolCall, Round: 2, Call: wait}})
	status := m.statusView()
	for _, want := range []string{"coordinating", "3 agents"} {
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

func TestAgentRunCellKeepsTaskAndFailureReason(t *testing.T) {
	started := time.Unix(100, 0)
	cell := AgentRunCell{AgentID: "dex-7", Profile: agent.ProfileIntelligence, Task: "review concurrency", State: agent.StateFailed, Reason: "timed out", StartedAt: started, FinishedAt: started.Add(30 * time.Second)}
	got := strings.Join(cell.RenderWidth(80), "\n")
	for _, want := range []string{"INT review concurrency", "timed out", "30.0s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("render=%q, want %q", got, want)
		}
	}
}

func TestAgentRunCellCompletedShowsSummary(t *testing.T) {
	started := time.Unix(200, 0)
	cell := AgentRunCell{AgentID: "int-2", Profile: agent.ProfileAgility, Task: "inspect router", State: agent.StateCompleted, Summary: "found routing boundary", StartedAt: started, FinishedAt: started.Add(8*time.Second + 400*time.Millisecond)}
	got := strings.Join(cell.RenderWidth(80), "\n")
	for _, want := range []string{"AGI inspect router", "found routing boundary", "8.4s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("render=%q, want %q", got, want)
		}
	}
}

func TestAgentRunCellRunningUsesActivityWithoutFakeDuration(t *testing.T) {
	cell := AgentRunCell{AgentID: "int-3", Profile: agent.ProfileAgility, Task: "trace cache", State: agent.StateRunning, Activity: `Search "routeRequest"`, StartedAt: time.Unix(300, 0), Spinner: "⠋"}
	got := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(got, "trace cache") || !strings.Contains(got, `Search "routeRequest"`) {
		t.Fatalf("render=%q", got)
	}
	if strings.Contains(got, "0s") {
		t.Fatalf("running cell exposed fake duration: %q", got)
	}
}

func TestAgentRunCellTerminalFallbacksAreExplicit(t *testing.T) {
	for _, tc := range []struct {
		state agent.State
		want  string
	}{{state: agent.StateCanceled, want: "canceled"}, {state: agent.StateFailed, want: "failed"}} {
		cell := AgentRunCell{Profile: agent.ProfileIntelligence, Task: "review concurrency", State: tc.state}
		got := strings.Join(cell.RenderWidth(80), "\n")
		if !strings.Contains(got, "review concurrency") || !strings.Contains(got, tc.want) {
			t.Fatalf("state=%s render=%q", tc.state, got)
		}
	}
}
