package agenttool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestResumeAgentStartsFreshChild(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockDelegateRunner{runFunc: func(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
				return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "resumed"}}, nil
			}}, nil
		}),
	)
	defer coord.Close()
	if err := coord.RestorePersistentSnapshot(agent.PersistentSnapshot{Version: agent.PersistentSnapshotVersion, Agents: []agent.PersistentAgent{{
		Status:  agent.AgentStatus{ID: "strength-2", ParentID: "old-turn", Profile: agent.ProfileStrength, Task: "finish fix", State: agent.StateRunning},
		Request: agent.Request{ID: "strength-2", ParentID: "old-turn", Profile: agent.ProfileStrength, Task: "finish fix"},
	}}}); err != nil {
		t.Fatal(err)
	}

	handler := NewResumeAgent(coord)
	call, _ := tool.NewCall("resume-1", "subagent", json.RawMessage(`{"agent_id":"strength-2"}`))
	ctx := agent.WithParentID(context.Background(), "new-turn")
	res, err := handler.Execute(ctx, call)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var payload struct {
		ResumedFrom string      `json:"resumed_from"`
		AgentID     string      `json:"agent_id"`
		Status      agent.State `json:"status"`
	}
	if err := json.Unmarshal(res.StructuredOutput, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ResumedFrom != "strength-2" || payload.AgentID == "" || payload.AgentID == payload.ResumedFrom {
		t.Fatalf("payload = %#v", payload)
	}
	if !strings.Contains(res.Output, "resumed strength-2") {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestResumeAgentRejectsCompletedRun(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	if err := coord.RestorePersistentSnapshot(agent.PersistentSnapshot{Version: agent.PersistentSnapshotVersion, Agents: []agent.PersistentAgent{{
		Status:  agent.AgentStatus{ID: "agility-1", Profile: agent.ProfileAgility, Task: "inspect", State: agent.StateCompleted},
		Request: agent.Request{ID: "agility-1", Profile: agent.ProfileAgility, Task: "inspect"},
	}}}); err != nil {
		t.Fatal(err)
	}
	handler := NewResumeAgent(coord)
	call, _ := tool.NewCall("resume-complete", "subagent", json.RawMessage(`{"agent_id":"agility-1"}`))
	_, err := handler.Execute(context.Background(), call)
	if err == nil || !strings.Contains(err.Error(), "not resumable") {
		t.Fatalf("Execute() error = %v", err)
	}
}
