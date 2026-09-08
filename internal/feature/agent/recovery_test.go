package agent

import (
	"context"
	"testing"
	"time"
)

func TestRecoverLifecycleReplaysCompletedAgentWithoutSnapshot(t *testing.T) {
	queuedAt := time.Now().UTC().Add(-time.Minute)
	req := Request{
		SessionID: "session-a", ParentID: "turn-1", ID: "agility-4",
		Profile: ProfileAgility, Task: "inspect router", Context: "focus on retries",
	}
	events := []LifecycleEvent{{
		Kind: LifecycleAgentQueued, Version: 1, At: queuedAt,
		SessionID: req.SessionID, ParentID: req.ParentID, AgentID: req.ID,
		Profile: req.Profile, Task: req.Task, Request: &req,
	}, {
		Kind: LifecycleAgentStarted, Version: 2, At: queuedAt.Add(time.Second),
		SessionID: req.SessionID, ParentID: req.ParentID, AgentID: req.ID,
		Profile: req.Profile, Task: req.Task,
	}}
	result := Result{SessionID: req.SessionID, AgentID: req.ID, Profile: req.Profile, Summary: "found retry path"}
	events = append(events, LifecycleEvent{
		Kind: LifecycleAgentCompleted, Version: 3, At: queuedAt.Add(2 * time.Second),
		SessionID: req.SessionID, ParentID: req.ParentID, AgentID: req.ID,
		Profile: req.Profile, Task: req.Task, Result: &result,
	})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	if err := coord.RecoverLifecycle(context.Background(), "session-a", nil, events); err != nil {
		t.Fatal(err)
	}
	status, got, ok := coord.LookupRef(AgentRef{SessionID: "session-a", AgentID: req.ID})
	if !ok || status.State != StateCompleted || status.Version != 3 {
		t.Fatalf("status = %+v ok=%v", status, ok)
	}
	if got == nil || got.Summary != "found retry path" {
		t.Fatalf("result = %+v", got)
	}
	coord.agentsMu.RLock()
	recoveredContext := coord.agents[req.ID].request.Context
	coord.agentsMu.RUnlock()
	if recoveredContext != req.Context {
		t.Fatalf("recovered context = %q", recoveredContext)
	}
}

func TestRecoverLifecycleInterruptsUnfinishedRun(t *testing.T) {
	store := &recordingLifecycleStore{}
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil, WithLifecycleEventStore(store))
	defer coord.Close()
	queuedAt := time.Now().UTC().Add(-time.Minute)
	events := []LifecycleEvent{{
		Kind: LifecycleAgentQueued, Version: 1, At: queuedAt,
		SessionID: "session-a", ParentID: "turn-2", AgentID: "strength-8",
		Profile: ProfileStrength, Task: "finish migration",
	}, {
		Kind: LifecycleAgentStarted, Version: 2, At: queuedAt.Add(time.Second),
		SessionID: "session-a", ParentID: "turn-2", AgentID: "strength-8",
		Profile: ProfileStrength, Task: "finish migration",
	}}
	if err := coord.RecoverLifecycle(context.Background(), "session-a", nil, events); err != nil {
		t.Fatal(err)
	}
	status, ok := coord.GetRef(AgentRef{SessionID: "session-a", AgentID: "strength-8"})
	if !ok || status.State != StateInterrupted || status.Version != 3 {
		t.Fatalf("status = %+v ok=%v", status, ok)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.events) != 1 || store.events[0].Kind != LifecycleAgentInterrupted || store.events[0].Version != 3 {
		t.Fatalf("persisted recovery events = %#v", store.events)
	}
}

func TestRecoverLifecycleReplaysJournalAfterSnapshotVersion(t *testing.T) {
	snapshot := PersistentSnapshot{Version: PersistentSnapshotVersion, Agents: []PersistentAgent{{
		Status: AgentStatus{
			SessionID: "session-a", ID: "agility-2", ParentID: "turn-3",
			Profile: ProfileAgility, Task: "inspect", State: StateRunning, Version: 2,
		},
		Request: Request{SessionID: "session-a", ID: "agility-2", ParentID: "turn-3", Profile: ProfileAgility, Task: "inspect"},
	}}}
	result := Result{SessionID: "session-a", AgentID: "agility-2", Profile: ProfileAgility, Summary: "done"}
	events := []LifecycleEvent{{
		Kind: LifecycleAgentQueued, Version: 1, At: time.Now().Add(-time.Minute),
		SessionID: "session-a", ParentID: "turn-3", AgentID: "agility-2", Profile: ProfileAgility, Task: "inspect",
	}, {
		Kind: LifecycleAgentStarted, Version: 2, At: time.Now().Add(-time.Second),
		SessionID: "session-a", ParentID: "turn-3", AgentID: "agility-2", Profile: ProfileAgility, Task: "inspect",
	}, {
		Kind: LifecycleAgentCompleted, Version: 3, At: time.Now(),
		SessionID: "session-a", ParentID: "turn-3", AgentID: "agility-2", Profile: ProfileAgility, Task: "inspect", Result: &result,
	}}
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	if err := coord.RecoverLifecycle(context.Background(), "session-a", &snapshot, events); err != nil {
		t.Fatal(err)
	}
	status, got, ok := coord.LookupRef(AgentRef{SessionID: "session-a", AgentID: "agility-2"})
	if !ok || status.State != StateCompleted || status.Version != 3 {
		t.Fatalf("status = %+v ok=%v", status, ok)
	}
	if got == nil || got.Summary != "done" {
		t.Fatalf("result = %+v", got)
	}
}
