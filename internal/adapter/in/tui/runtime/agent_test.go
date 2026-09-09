package runtime

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
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

func TestDisabledSubagentsAppearInFooterAndAgentsPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.subagentsEnabled = false
	if got := m.infoView(); strings.Contains(got, "subagents") {
		t.Fatalf("minimal info view leaked subagent state: %q", got)
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

func TestStatusViewShowsSubagentCoordinationDuringBusyTurn(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(80, 24)
	m.busy = true
	m.busyStarted = time.Now().Add(-4 * time.Second)
	m.activity = "thinking"
	m.agentSnapshot = []agent.AgentStatus{{ID: "explorer-1", State: agent.StateRunning}, {ID: "reviewer-2", State: agent.StateQueued}}
	got := m.statusView()
	if !strings.Contains(got, "working") || !strings.Contains(got, "2 agents") {
		t.Fatalf("status view=%q", got)
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
	for _, want := range []string{"working", "1 agent"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status view=%q, want %q", got, want)
		}
	}
}

func TestApplyTurnEventTracksRoundAndToolCount(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Round: 2, Call: tool.Call{ID: "c1", Name: "read"}})
	if m.turnProgress.Round != 2 || m.turnProgress.ToolCalls != 1 {
		t.Fatalf("turn progress=%+v, want round 2 and 1 tool", m.turnProgress)
	}
}

func TestStatusViewAvoidsDuplicatingAgentPaneDetail(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(140, 30)
	m.busy = true
	m.agentSnapshot = []agent.AgentStatus{{ID: "explorer-1", State: agent.StateRunning}, {ID: "reviewer-2", State: agent.StateQueued}}
	got := m.statusView()
	if !strings.Contains(got, "2 agents working") {
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
	if got := m.statusView(); !strings.Contains(got, "stopping 1 agent") {
		t.Fatalf("status=%q", got)
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
	delegate, _ := tool.NewCall("d1", "subagent", json.RawMessage(`{"action":"spawn","profile":"int","task":"inspect router"}`))
	wait, _ := tool.NewCall("w1", "subagent", json.RawMessage(`{"action":"wait"}`))
	m.applyTurnEvents([]turn.Event{{Kind: turn.EventToolCall, Call: delegate}, {Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "subagent", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"queued"}`)}}, {Kind: turn.EventToolCall, Call: wait}, {Kind: turn.EventToolResult, Call: wait, Result: tool.Result{CallID: "w1", ToolName: "subagent", StructuredOutput: json.RawMessage(`{"timed_out":false,"event":{"kind":"agent_completed","agent_id":"int-7","message":"found routing issue"},"agents":[{"id":"int-7","state":"completed"}]}`)}}})
	cells := m.historyState.Cells()
	if len(cells) != 1 {
		t.Fatalf("history cells=%d, want one delegated run: %#v", len(cells), cells)
	}
	run, ok := cells[0].(*AgentRunCell)
	if !ok || run.AgentID != "int-7" || run.Task != "inspect router" || run.Summary != "found routing issue" {
		t.Fatalf("run=%T %#v", cells[0], cells[0])
	}
	plain := strings.Join(run.RawLines(), "\n")
	for _, leaked := range []string{"Waiting for", "Checking subagents"} {
		if strings.Contains(plain, leaked) {
			t.Fatalf("orchestration detail leaked into run cell: %q", plain)
		}
	}
}

func TestSubagentRunsStayDistinctByAgentID(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	for _, tc := range []struct{ callID, agentID, task string }{{"d1", "int-1", "inspect router"}, {"d2", "int-2", "inspect cache"}} {
		call, _ := tool.NewCall(tc.callID, "subagent", json.RawMessage(`{"action":"spawn","profile":"int","task":"`+tc.task+`"}`))
		m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: call})
		m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: call, Result: tool.Result{CallID: tc.callID, ToolName: "subagent", StructuredOutput: json.RawMessage(`{"agent_id":"` + tc.agentID + `","status":"queued"}`)}})
	}
	if m.historyState.AgentRun("int-1") == nil || m.historyState.AgentRun("int-2") == nil {
		t.Fatalf("parallel runs were conflated: %#v", m.historyState.Cells())
	}
	if len(m.historyState.Cells()) != 2 {
		t.Fatalf("cells=%d, want 2", len(m.historyState.Cells()))
	}
}

func TestAgentWaitTimeoutDoesNotLeakRPCTranscript(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	delegate, _ := tool.NewCall("d1", "subagent", json.RawMessage(`{"action":"spawn","profile":"int","task":"inspect router"}`))
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: delegate})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "subagent", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"running"}`)}})
	wait, _ := tool.NewCall("w1", "subagent", json.RawMessage(`{"action":"wait"}`))
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: wait})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: wait, Result: tool.Result{CallID: "w1", ToolName: "subagent", StructuredOutput: json.RawMessage(`{"timed_out":true,"event":null,"agents":[{"id":"int-7","state":"running"}]}`)}})
	plain := m.historyState.Raw()
	for _, leaked := range []string{"Wait agent", "Waiting for int-7", "deadline_exceeded", "wait timed out"} {
		if strings.Contains(plain, leaked) {
			t.Fatalf("orchestration wait leaked into transcript: %q", plain)
		}
	}
	run := m.historyState.AgentRun("int-7")
	if run == nil || run.Activity != "" || run.State != agent.StateRunning {
		t.Fatalf("run state=%#v", run)
	}
}

func TestOutOfOrderAgentResultMergesIntoDelegateRun(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	delegate, _ := tool.NewCall("d1", "subagent", json.RawMessage(`{"action":"spawn","profile":"int","task":"inspect router"}`))
	get, _ := tool.NewCall("g1", "subagent", json.RawMessage(`{"action":"get"}`))
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: delegate})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: get})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: get, Result: tool.Result{CallID: "g1", ToolName: "subagent", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"running"}`)}})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "subagent", StructuredOutput: json.RawMessage(`{"agent_id":"int-7","status":"running"}`)}})
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
	delegate, _ := tool.NewCall("d1", "subagent", json.RawMessage(`{"action":"spawn","profile":"dex","task":"review concurrency"}`))
	cancel, _ := tool.NewCall("c1", "subagent", json.RawMessage(`{"action":"cancel","agent_id":"dex-7"}`))
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: delegate})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d1", ToolName: "subagent", StructuredOutput: json.RawMessage(`{"agent_id":"dex-7","status":"running"}`)}})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: cancel})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: cancel, Result: tool.Result{CallID: "c1", ToolName: "subagent", StructuredOutput: json.RawMessage(`{"agent_id":"dex-7","status":"canceled"}`)}})
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
	delegate, _ := tool.NewCall("d-missing", "subagent", json.RawMessage(`{"action":"spawn","profile":"int","task":"inspect router"}`))
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolCall, Call: delegate})
	m.applyTurnEvent(turn.Event{Kind: turn.EventToolResult, Call: delegate, Result: tool.Result{CallID: "d-missing", ToolName: "subagent", StructuredOutput: json.RawMessage(`{"status":"queued"}`)}})
	cells := m.historyState.Cells()
	if len(cells) != 1 {
		t.Fatalf("missing agent id corrupted history: %#v", cells)
	}
	if run, ok := cells[0].(*AgentRunCell); ok && run.AgentID == "" {
		t.Fatalf("malformed response created unaddressable run cell: %#v", run)
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

func TestAgentProgressKeepsFrameWithinTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.busy = true
	m.agentSnapshot = []agent.AgentStatus{{ID: "worker-1", Profile: agent.ProfileStrength, Task: "fix failures", State: agent.StateRunning, StartedAt: time.Now()}}
	m.relayout()
	if got := lipgloss.Height(m.View().Content); got > m.height {
		t.Fatalf("initial frame height=%d terminal=%d", got, m.height)
	}
	call, _ := tool.NewCall("grep-1", "grep", []byte(`{"pattern":"TDZ","path":"."}`))
	updated, _ := m.Update(agentLifecycleMsg{event: agent.Event{Kind: agent.EventAgentProgress, AgentID: "worker-1", Call: &call}})
	m = updated.(*bubbleModel)
	if got := lipgloss.Height(m.View().Content); got > m.height {
		t.Fatalf("agent progress frame height=%d terminal=%d", got, m.height)
	}
}

func TestRelayoutDoesNotReenableFollowTailAfterUserScroll(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.showWelcome = false
	m.resize(80, 24)
	for i := 0; i < 80; i++ {
		m.appendLine(fmt.Sprintf("line-%02d", i))
	}
	m.todo = []TodoItem{{ID: "a", Text: "dynamic chrome", Status: tododomain.StatusInProgress}}
	m.relayout()
	m.viewport.GotoBottom()
	m.viewport.ScrollUp(1)
	m.followTail = false
	m.todo = nil
	m.relayout()
	if m.followTail {
		t.Fatal("relayout re-enabled follow tail after explicit user scroll")
	}
}

func TestRefreshViewportPreservesLogicalAnchorAcrossCellExpansion(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.showWelcome = false
	m.resize(80, 16)
	run := &AgentRunCell{AgentID: "worker-1", Profile: agent.ProfileStrength, Task: "fix failures", State: agent.StateRunning}
	m.historyState.Append(run)
	for i := 0; i < 40; i++ {
		m.appendLine(fmt.Sprintf("line-%02d", i))
	}
	m.refreshViewport()
	m.viewport.SetYOffset(10)
	m.followTail = false
	before := strings.Split(ansi.Strip(m.viewport.View()), "\n")[0]
	run.Activity = "search TDZ"
	m.historyState.TouchAgentRun("worker-1")
	m.refreshViewport()
	after := strings.Split(ansi.Strip(m.viewport.View()), "\n")[0]
	if after != before {
		t.Fatalf("logical scroll anchor moved: before=%q after=%q", before, after)
	}
	if m.followTail {
		t.Fatal("content expansion re-enabled follow tail")
	}
}

func TestScrolledViewportSurvivesLiveAgentChromeStress(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.showWelcome = false
	m.resize(90, 24)
	run := &AgentRunCell{AgentID: "worker-1", Profile: agent.ProfileStrength, Task: "fix TDZ and bun adapter", State: agent.StateRunning, StartedAt: time.Now()}
	m.historyState.Append(run)
	for i := 0; i < 80; i++ {
		m.appendLine(fmt.Sprintf("history-%02d", i))
	}
	m.busy = true
	m.busyStarted = time.Now().Add(-5 * time.Minute)
	m.agentSnapshot = []agent.AgentStatus{{ID: "worker-1", Profile: agent.ProfileStrength, Task: "fix TDZ and bun adapter", State: agent.StateRunning, StartedAt: time.Now().Add(-5 * time.Minute)}}
	m.relayout()
	m.viewport.GotoBottom()
	m.viewport.ScrollUp(7)
	m.followTail = false
	firstSemanticLine := func() string {
		for _, line := range strings.Split(ansi.Strip(m.viewport.View()), "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				return trimmed
			}
		}
		return ""
	}
	firstVisible := firstSemanticLine()

	assertStable := func(stage string) {
		t.Helper()
		view := m.View().Content
		if got := lipgloss.Height(view); got > m.height {
			t.Fatalf("%s frame height=%d terminal=%d", stage, got, m.height)
		}
		for _, line := range strings.Split(view, "\n") {
			if got := ansi.StringWidth(line); got > m.width {
				t.Fatalf("%s line width=%d terminal=%d: %q", stage, got, m.width, line)
			}
		}
		if m.followTail {
			t.Fatalf("%s unexpectedly re-enabled follow tail", stage)
		}
		if got := firstSemanticLine(); got != firstVisible {
			t.Fatalf("%s moved logical anchor: before=%q after=%q", stage, firstVisible, got)
		}
		if strings.Contains(ansi.Strip(m.historyState.RenderContent()), "1 agent working") {
			t.Fatalf("%s persisted ephemeral coordination status into history", stage)
		}
	}

	run.Activity = "กำลังแยกกลุ่ม failure ว่าเป็น TDZ, bun-adapter, หรือ logic จริง"
	m.agentActivity["worker-1"] = AgentActivity{Label: run.Activity}
	m.historyState.TouchAgentRun("worker-1")
	m.relayout()
	assertStable("thai agent progress")

	updated, _ := m.Update(spinner.TickMsg{})
	m = updated.(*bubbleModel)
	assertStable("spinner tick")

	m.todo = []TodoItem{{ID: "fix", Text: "ตรวจสอบผลแก้ไข", Status: tododomain.StatusInProgress}}
	m.relayout()
	assertStable("todo expanded")
	m.todo = nil
	m.relayout()
	assertStable("todo collapsed")

	m.agentSnapshot = nil
	run.State = agent.StateCompleted
	run.FinishedAt = time.Now()
	m.historyState.TouchAgentRun("worker-1")
	m.relayout()
	assertStable("agent completed")

	for !m.viewport.AtBottom() {
		updated, _ = m.Update(testKey(tea.KeyPgDown))
		m = updated.(*bubbleModel)
	}
	if !m.followTail {
		t.Fatal("explicit page down to bottom did not re-enable follow tail")
	}
}
