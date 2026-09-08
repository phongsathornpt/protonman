package sessionfs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestFileStoreRoundTripsAgentSnapshot(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := agent.PersistentSnapshot{Version: agent.PersistentSnapshotVersion, UpdatedAt: time.Now().UTC(), Agents: []agent.PersistentAgent{{
		Status:  agent.AgentStatus{ID: "agility-4", ParentID: "turn-1", Profile: agent.ProfileAgility, Task: "inspect router", State: agent.StateCompleted, FinishedAt: time.Now().UTC()},
		Request: agent.Request{ID: "agility-4", ParentID: "turn-1", Profile: agent.ProfileAgility, Task: "inspect router"},
		Result:  &agent.Result{AgentID: "agility-4", Profile: agent.ProfileAgility, Summary: "found duplicate branch"},
	}}}
	if err := store.SaveAgents(context.Background(), "session-1", want); err != nil {
		t.Fatalf("SaveAgents() error = %v", err)
	}
	got, found, err := store.LoadAgents(context.Background(), "session-1")
	if err != nil || !found {
		t.Fatalf("LoadAgents() found=%v err=%v", found, err)
	}
	if len(got.Agents) != 1 || got.Agents[0].Result == nil || got.Agents[0].Result.Summary != "found duplicate branch" {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestFileStoreAgentSnapshotUsesPrivateFile(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAgents(context.Background(), "private", agent.PersistentSnapshot{Version: agent.PersistentSnapshotVersion}); err != nil {
		t.Fatal(err)
	}
	resources, err := session.ResolveResources(root, "private")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(resources.Agents)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}

func TestFileStoreLoadAgentsMissingIsNotFound(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	_, found, err := store.LoadAgents(context.Background(), "missing")
	if err != nil || found {
		t.Fatalf("LoadAgents() found=%v err=%v", found, err)
	}
}
