package tui

import (
	"context"
	"strings"
	"testing"

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
