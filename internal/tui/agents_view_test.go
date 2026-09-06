package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
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
	if _, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileExplorer, Task: "inspect router"}); err != nil {
		t.Fatal(err)
	}

	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.coordinator = coord
	m.agentSnapshot = coord.List()
	m.resize(80, 24)
	if got := m.agentsView(); !strings.Contains(got, "Agents 1 active") || !strings.Contains(got, "inspect router") {
		t.Fatalf("agents view=%q", got)
	}
	m.resize(60, 18)
	if got := m.agentsView(); !strings.Contains(got, "Agents 1 active") || strings.Contains(got, "inspect router") {
		t.Fatalf("compact agents view=%q", got)
	}
	m.resize(24, 12)
	if got := m.agentsView(); got != "" {
		t.Fatalf("tiny agents view=%q", got)
	}
	close(release)
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
	m.coordinator = coord
	m.agentEvents = events
	if _, err := coord.Spawn(context.Background(), agent.Request{Profile: agent.ProfileExplorer, Task: "inspect router"}); err != nil {
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
		{ID: "run-1", Task: "inspect active", State: agent.StateRunning, StartedAt: now.Add(-3 * time.Second)},
		{ID: "cancel-1", Task: "stop active", State: agent.StateCanceling, StartedAt: now.Add(-4 * time.Second)},
	}
	m.resize(100, 30)
	got := m.agentsView()
	if !strings.Contains(got, "Agents 2 active") || !strings.Contains(got, "1 running") || !strings.Contains(got, "1 canceling") {
		t.Fatalf("agents view summary=%q", got)
	}
	if !strings.Contains(got, "run-1") || !strings.Contains(got, "cancel-1") {
		t.Fatalf("active agents were hidden by terminal rows: %q", got)
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
	updated, _ := m.Update(agentLifecycleMsg{event: agent.Event{Kind: agent.EventAgentProgress, AgentID: "explorer-1", Message: "using grep"}})
	m = updated.(*bubbleModel)
	m.agentSnapshot = []agent.AgentStatus{{ID: "explorer-1", Task: "inspect router", State: agent.StateRunning, StartedAt: time.Now()}}
	if got := m.agentsView(); !strings.Contains(got, "using grep") {
		t.Fatalf("agents view=%q, want current activity", got)
	}
	updated, _ = m.Update(agentLifecycleMsg{event: agent.Event{Kind: agent.EventAgentCompleted, AgentID: "explorer-1"}})
	m = updated.(*bubbleModel)
	if _, ok := m.agentActivity["explorer-1"]; ok {
		t.Fatal("terminal lifecycle event did not clear transient activity")
	}
}

func TestAgentsViewCollapsesDuringBusyRootTurn(t *testing.T) {
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
	if strings.Contains(got, "inspect router") || strings.Contains(got, "review risks") {
		t.Fatalf("busy agents view should be collapsed: %q", got)
	}
}
