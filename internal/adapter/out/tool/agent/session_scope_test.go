package agenttool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func sessionCtx(sessionID, turnID string) context.Context {
	return agent.WithTurnRef(context.Background(), agent.TurnRef{SessionID: sessionID, TurnID: turnID})
}

func TestLifecycleToolsEnforceSessionOwnership(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	for _, sessionID := range []string{"session-a", "session-b"} {
		if err := coord.RestorePersistentSnapshot(agent.PersistentSnapshot{Version: agent.PersistentSnapshotVersion, Agents: []agent.PersistentAgent{{
			Status:  agent.AgentStatus{SessionID: sessionID, ID: "agent-" + sessionID, Profile: agent.ProfileAgility, Task: "inspect", State: agent.StateCompleted},
			Request: agent.Request{SessionID: sessionID, ID: "agent-" + sessionID, Profile: agent.ProfileAgility, Task: "inspect"},
		}}}); err != nil {
			t.Fatal(err)
		}
	}
	listCall, _ := tool.NewCall("list-a", "subagent", json.RawMessage(`{}`))
	list, err := NewListAgents(coord).Execute(sessionCtx("session-a", "turn-1"), listCall)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(list.StructuredOutput), "agent-session-a") || strings.Contains(string(list.StructuredOutput), "agent-session-b") {
		t.Fatalf("session-a list leaked ownership: %s", list.StructuredOutput)
	}

	getCall, _ := tool.NewCall("get-b", "subagent", json.RawMessage(`{"agent_id":"agent-session-b"}`))
	if _, err := NewGetAgent(coord).Execute(sessionCtx("session-a", "turn-1"), getCall); err == nil {
		t.Fatal("session-a read session-b agent without error")
	}

	cancelCall, _ := tool.NewCall("cancel-b", "subagent", json.RawMessage(`{"agent_id":"agent-session-b"}`))
	if _, err := NewCancelAgent(coord).Execute(sessionCtx("session-a", "turn-1"), cancelCall); err == nil {
		t.Fatal("session-a canceled session-b agent without error")
	}
}
func TestResumeAgentCannotCrossSessionBoundary(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	if err := coord.RestorePersistentSnapshot(agent.PersistentSnapshot{Version: agent.PersistentSnapshotVersion, Agents: []agent.PersistentAgent{{
		Status:  agent.AgentStatus{SessionID: "session-b", ID: "strength-9", Profile: agent.ProfileStrength, Task: "finish", State: agent.StateInterrupted},
		Request: agent.Request{SessionID: "session-b", ID: "strength-9", Profile: agent.ProfileStrength, Task: "finish"},
	}}}); err != nil {
		t.Fatal(err)
	}
	call, _ := tool.NewCall("resume-b", "subagent", json.RawMessage(`{"agent_id":"strength-9"}`))
	if _, err := NewResumeAgent(coord).Execute(sessionCtx("session-a", "turn-1"), call); err == nil {
		t.Fatal("session-a resumed session-b agent without error")
	}
	status, ok := coord.GetRef(agent.AgentRef{SessionID: "session-b", AgentID: "strength-9"})
	if !ok || status.State != agent.StateInterrupted {
		t.Fatalf("cross-session resume mutated source: %#v", status)
	}
}
