package sessionfs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestFileStoreAppendsAndLoadsLifecycleEvents(t *testing.T) {
	root := t.TempDir()
	store, err := NewFileStore(root)
	if err != nil {
		t.Fatal(err)
	}
	first := agent.LifecycleEvent{
		Kind: agent.LifecycleAgentQueued, Version: 1, At: time.Now().UTC(),
		SessionID: "session-1", ParentID: "turn-1", AgentID: "agility-1",
		Profile: agent.ProfileAgility, Task: "inspect",
	}
	second := first
	second.Kind = agent.LifecycleAgentStarted
	second.Version = 2
	if err := store.AppendLifecycleEvent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendLifecycleEvent(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	events, err := store.LoadLifecycleEvents(context.Background(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Kind != agent.LifecycleAgentQueued || events[1].Version != 2 {
		t.Fatalf("events = %#v", events)
	}
	resources, err := session.ResolveResources(root, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(resources.AgentEvents)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal permissions = %o", info.Mode().Perm())
	}
}

func TestFileStoreMissingLifecycleJournalIsEmpty(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.LoadLifecycleEvents(context.Background(), "missing-session")
	if err != nil || len(events) != 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}
