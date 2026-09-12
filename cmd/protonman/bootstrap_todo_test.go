package main

import (
	"context"
	"os"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/session"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

func TestReconcileDelegatedTaskStatusUsesOwningSessionStore(t *testing.T) {
	ctx := context.Background()
	sessionsRoot := t.TempDir()
	stores := make(map[string]*tododomain.MarkdownStore)
	for _, sessionID := range []string{"session-a", "session-b"} {
		resources, err := session.ResolveResources(sessionsRoot, sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(resources.Root, 0o700); err != nil {
			t.Fatal(err)
		}
		store, err := tododomain.OpenMarkdownStore(ctx, resources.Todo)
		if err != nil {
			t.Fatal(err)
		}
		bound, _, err := store.BindGoal(ctx, "finish session work")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CompareAndReplace(ctx, bound.Revision, []tododomain.Item{{ID: "linked", Text: "linked task", Status: tododomain.StatusPending}}); err != nil {
			t.Fatal(err)
		}
		stores[sessionID] = store
	}

	if err := reconcileDelegatedTaskStatus(ctx, sessionsRoot, "session-b", "linked", tododomain.StatusCompleted); err != nil {
		t.Fatal(err)
	}
	for sessionID, want := range map[string]tododomain.Status{"session-a": tododomain.StatusPending, "session-b": tododomain.StatusCompleted} {
		if _, err := stores[sessionID].Reload(ctx); err != nil {
			t.Fatal(err)
		}
		got := stores[sessionID].Snapshot().Items[0].Status
		if got != want {
			t.Fatalf("%s task status = %q, want %q", sessionID, got, want)
		}
	}
}
