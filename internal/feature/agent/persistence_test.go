package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

func TestPersistentSnapshotRestoresLiveRunAsInterrupted(t *testing.T) {
	coord := NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()

	snapshot := PersistentSnapshot{Version: PersistentSnapshotVersion, Agents: []PersistentAgent{{
		Status:  AgentStatus{ID: "strength-7", ParentID: "turn-1", Profile: ProfileStrength, Task: "finish refactor", State: StateRunning, StartTime: time.Now().Add(-time.Minute)},
		Request: Request{ID: "strength-7", ParentID: "turn-1", Profile: ProfileStrength, Task: "finish refactor", Context: "focus on router"},
	}}}
	if err := coord.RestorePersistentSnapshot(snapshot); err != nil {
		t.Fatalf("RestorePersistentSnapshot() error = %v", err)
	}
	status, ok := coord.Get("strength-7")
	if !ok || status.State != StateInterrupted || !status.State.Terminal() {
		t.Fatalf("restored status = %#v, ok=%v", status, ok)
	}
	if !strings.Contains(status.Reason, "previous process") {
		t.Fatalf("reason = %q", status.Reason)
	}
	if coord.seq < 7 {
		t.Fatalf("sequence = %d, want >= 7", coord.seq)
	}
}

func TestPersistentSnapshotBoundsRequestText(t *testing.T) {
	coord := NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	coord.agents["agility-1"] = &agentEntry{
		status:  AgentStatus{ID: "agility-1", Profile: ProfileAgility, Task: strings.Repeat("t", maxPersistentTaskBytes+20), State: StateInterrupted},
		request: Request{ID: "agility-1", Profile: ProfileAgility, Task: strings.Repeat("t", maxPersistentTaskBytes+20), Context: strings.Repeat("c", maxPersistentContextBytes+20)},
		cancel:  func() {}, done: closedTestChannel(), started: closedTestChannel(),
	}
	snapshot := coord.PersistentSnapshot()
	if len(snapshot.Agents) != 1 {
		t.Fatalf("agents = %d, want 1", len(snapshot.Agents))
	}
	record := snapshot.Agents[0]
	if len(record.Request.Task) != maxPersistentTaskBytes || len(record.Request.Context) != maxPersistentContextBytes {
		t.Fatalf("persisted lengths task=%d context=%d", len(record.Request.Task), len(record.Request.Context))
	}
	if record.Status.Task != record.Request.Task {
		t.Fatal("status task and persisted request task diverged")
	}
}

func closedTestChannel() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func TestResumeStartsFreshChildFromInterruptedRecord(t *testing.T) {
	coord := verificationCoordinator(turn.Result{})
	defer coord.Close()
	snapshot := PersistentSnapshot{Version: PersistentSnapshotVersion, Agents: []PersistentAgent{{
		Status:  AgentStatus{ID: "strength-3", ParentID: "old-turn", Profile: ProfileStrength, Task: "finish migration", State: StateRunning},
		Request: Request{ID: "strength-3", ParentID: "old-turn", Profile: ProfileStrength, Task: "finish migration", Context: "check adapters"},
	}}}
	if err := coord.RestorePersistentSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	handle, err := coord.Resume(context.Background(), "strength-3", "new-turn")
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if handle.ID == "strength-3" {
		t.Fatal("resume reused interrupted agent id")
	}
	coord.agentsMu.RLock()
	req := coord.agents[handle.ID].request
	coord.agentsMu.RUnlock()
	if req.ParentID != "new-turn" || !strings.Contains(req.Context, "Re-inspect current state") || !strings.Contains(req.Context, "check adapters") {
		t.Fatalf("resumed request = %#v", req)
	}
}
