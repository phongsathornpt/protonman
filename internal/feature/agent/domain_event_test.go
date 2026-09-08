package agent

import (
	"errors"
	"testing"
	"time"
)

func TestApplyLifecycleEventBuildsVersionedProjection(t *testing.T) {
	queuedAt := time.Now().UTC()
	queued := LifecycleEvent{
		Kind: LifecycleAgentQueued, Version: 1, At: queuedAt,
		SessionID: "session-a", ParentID: "turn-1", AgentID: "agility-1",
		Profile: ProfileAgility, Task: "inspect router",
	}
	status, err := applyLifecycleEvent(AgentStatus{}, queued)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateQueued || status.Version != 1 || status.SessionID != "session-a" {
		t.Fatalf("queued projection = %+v", status)
	}

	startedAt := queuedAt.Add(time.Second)
	started := nextLifecycleEvent(status, LifecycleAgentStarted, startedAt, "")
	status, err = applyLifecycleEvent(status, started)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateRunning || status.Version != 2 || !status.StartedAt.Equal(startedAt) {
		t.Fatalf("running projection = %+v", status)
	}
}
func TestApplyLifecycleEventRejectsStaleVersionAndWrongSession(t *testing.T) {
	base := AgentStatus{
		SessionID: "session-a", ID: "strength-2", ParentID: "turn-2",
		Profile: ProfileStrength, Task: "fix", State: StateRunning, Version: 4,
	}
	stale := nextLifecycleEvent(base, LifecycleAgentCompleted, time.Now(), "")
	stale.Version = 4
	if _, err := applyLifecycleEvent(base, stale); err == nil {
		t.Fatal("expected stale lifecycle version conflict")
	}

	wrongSession := nextLifecycleEvent(base, LifecycleAgentCompleted, time.Now(), "")
	wrongSession.SessionID = "session-b"
	if _, err := applyLifecycleEvent(base, wrongSession); err == nil {
		t.Fatal("expected lifecycle session mismatch")
	}
}

func TestApplyLifecycleEventRejectsInvalidTransition(t *testing.T) {
	base := AgentStatus{
		SessionID: "session-a", ID: "agility-3", Profile: ProfileAgility,
		Task: "inspect", State: StateCompleted, Version: 2,
	}
	event := nextLifecycleEvent(base, LifecycleAgentStarted, time.Now(), "")
	_, err := applyLifecycleEvent(base, event)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("applyLifecycleEvent() error = %v, want ErrInvalidTransition", err)
	}
}
