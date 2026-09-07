package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/adapter/out/model"
	"github.com/projectTHORN/proton/internal/app"
	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/engine/toolcall"
	"github.com/projectTHORN/proton/internal/engine/turn"
	"github.com/projectTHORN/proton/internal/feature/agent"
)

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
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return blockingAgentViewRunner{release: release}, nil
		}),
	)
	defer coord.Close()
	if _, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileINT, Task: "inspect router"}); err != nil {
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
	m.agentSnapshot = []agent.AgentStatus{{ID: "int-1", Profile: agent.ProfileINT, Task: "inspect", State: agent.StateRunning, StartedAt: time.Now()}}
	joined := strings.Join(agentInspectionRows(m), "\n")
	if !strings.Contains(joined, "New delegation disabled") || !strings.Contains(joined, "INT") {
		t.Fatalf("agents pane=%q", joined)
	}
}

func TestAgentLifecycleMessageRefreshesSnapshot(t *testing.T) {
	release := make(chan struct{})
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return blockingAgentViewRunner{release: release}, nil
		}),
	)
	defer coord.Close()
	events, cancel := coord.Subscribe(8)
	defer cancel()
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.agents = app.NewAgents(coord)
	m.agentEvents = events
	if _, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileINT, Task: "inspect router"}); err != nil {
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
	m.agentSnapshot = []agent.AgentStatus{
		{ID: "done-1", Task: "old result", State: agent.StateCompleted, StartedAt: now.Add(-20 * time.Second), FinishedAt: now.Add(-15 * time.Second)},
		{ID: "done-2", Task: "new result", State: agent.StateCompleted, StartedAt: now.Add(-10 * time.Second), FinishedAt: now.Add(-9 * time.Second)},
		{ID: "run-1", Profile: agent.ProfileINT, Task: "inspect active", State: agent.StateRunning, StartedAt: now.Add(-3 * time.Second)},
		{ID: "cancel-1", Profile: agent.ProfileDEX, Task: "stop active", State: agent.StateCanceling, StartedAt: now.Add(-4 * time.Second)},
	}
	m.resize(100, 30)
	got := m.agentsView()
	if !strings.Contains(got, "Agents 2 active") || !strings.Contains(got, "1 running") || !strings.Contains(got, "1 canceling") {
		t.Fatalf("agents view summary=%q", got)
	}
	if !strings.Contains(got, "INT") || !strings.Contains(got, "DEX") || strings.Contains(got, "run-1") || strings.Contains(got, "cancel-1") {
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
	m.agentSnapshot = []agent.AgentStatus{
		{ID: "explorer-1", State: agent.StateRunning},
		{ID: "reviewer-2", State: agent.StateQueued},
	}
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
	m.agentSnapshot = []agent.AgentStatus{
		{ID: "explorer-1", Task: "inspect router", State: agent.StateRunning, StartedAt: time.Now()},
		{ID: "reviewer-2", Task: "review risks", State: agent.StateQueued},
	}
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
	m.agentSnapshot = []agent.AgentStatus{{
		ID: "reviewer-2", Task: "review security", State: agent.StateFailed,
		StartedAt: time.Now().Add(-10 * time.Second), FinishedAt: time.Now(), Reason: "timed out",
	}}
	if got := m.agentsView(); got != "" {
		t.Fatalf("terminal agent leaked into live pane: %q", got)
	}
}

func TestStatusViewAvoidsDuplicatingAgentPaneDetail(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(140, 30)
	m.busy = true
	m.agentSnapshot = []agent.AgentStatus{
		{ID: "explorer-1", State: agent.StateRunning},
		{ID: "reviewer-2", State: agent.StateQueued},
	}
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
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithMaxConcurrency(2),
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return blockingAgentViewRunner{release: release}, nil
		}),
	)
	defer func() { close(release); _ = coord.Close() }()

	owned, err := coord.Spawn(context.Background(), agent.Request{ParentID: "turn-owned", Profile: agent.ProfileINT, Task: "owned"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := coord.Spawn(context.Background(), agent.Request{ParentID: "turn-other", Profile: agent.ProfileINT, Task: "other"})
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
	m.turnCancel = func() { cancelled = true }

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
	m.agentSnapshot = []agent.AgentStatus{
		{ID: "current", ParentID: "turn-current", Task: "current task", State: agent.StateRunning},
		{ID: "old", ParentID: "turn-old", Task: "old task", State: agent.StateRunning},
	}
	got := m.agentsView()
	if !strings.Contains(got, "Agents 1 active") || strings.Contains(got, "old task") {
		t.Fatalf("agents view=%q", got)
	}
}

func TestAgentsCommandOpensFocusedInspectionPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(100, 30)
	m.agentSnapshot = []agent.AgentStatus{
		{ID: "int-7", Profile: agent.ProfileINT, Task: "inspect router", State: agent.StateRunning, StartedAt: time.Now().Add(-4 * time.Second)},
		{ID: "dex-8", Profile: agent.ProfileDEX, Task: "review concurrency", State: agent.StateFailed, StartedAt: time.Now().Add(-6 * time.Second), FinishedAt: time.Now(), Reason: "timed out"},
	}
	m.agentActivity["int-7"] = AgentActivity{Label: `Search "routeRequest"`}

	_ = m.executeCommand("/agents")
	pane := m.bottom.find(agentsViewID)
	if pane == nil {
		t.Fatal("/agents did not open inspection pane")
	}
	got := pane.Render(m)
	for _, want := range []string{"INT", "inspect router", "int-7", "DEX", "review concurrency", "dex-8", "timed out"} {
		if !strings.Contains(got, want) {
			t.Fatalf("agents pane=%q, want %q", got, want)
		}
	}
}
