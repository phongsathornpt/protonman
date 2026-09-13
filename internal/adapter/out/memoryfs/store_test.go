package memoryfs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/memory"
)

func TestFileStoreKeepsWorkspaceAndGlobalIndexesIsolated(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	workspace := memory.Entry{ID: "workspace", Scope: memory.ScopeWorkspace, Kind: memory.KindRepoFact, Key: "test", Value: "go test ./...", WorkspaceKey: "abc123", Confidence: 1}
	global := memory.Entry{ID: "global", Scope: memory.ScopeGlobal, Kind: memory.KindPreference, Key: "style", Value: "prefer focused changes", Confidence: 0.9}
	if err := store.Replace(ctx, memory.ScopeWorkspace, "abc123", []memory.Entry{workspace}); err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(ctx, memory.ScopeGlobal, "", []memory.Entry{global}); err != nil {
		t.Fatal(err)
	}
	workspaceEntries, err := store.Load(ctx, memory.ScopeWorkspace, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	globalEntries, err := store.Load(ctx, memory.ScopeGlobal, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaceEntries) != 1 || workspaceEntries[0].ID != "workspace" {
		t.Fatalf("workspace entries = %+v", workspaceEntries)
	}
	if len(globalEntries) != 1 || globalEntries[0].ID != "global" {
		t.Fatalf("global entries = %+v", globalEntries)
	}
}

func TestFileStoreRecordUsageUpdatesOnlySelectedEntry(t *testing.T) {
	root := t.TempDir()
	store, _ := NewFileStore(root)
	ctx := context.Background()
	entries := []memory.Entry{
		{ID: "a", Scope: memory.ScopeWorkspace, Kind: memory.KindProcedure, Key: "a", Value: "one", WorkspaceKey: "abc123", Confidence: 1},
		{ID: "b", Scope: memory.ScopeWorkspace, Kind: memory.KindProcedure, Key: "b", Value: "two", WorkspaceKey: "abc123", Confidence: 1},
	}
	if err := store.Replace(ctx, memory.ScopeWorkspace, "abc123", entries); err != nil {
		t.Fatal(err)
	}
	usedAt := time.Date(2026, 9, 13, 3, 0, 0, 0, time.UTC)
	if err := store.RecordUsage(ctx, []memory.UsageRef{{Scope: memory.ScopeWorkspace, WorkspaceKey: "abc123", ID: "b"}}, usedAt); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx, memory.ScopeWorkspace, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if loaded[0].UsageCount != 0 {
		t.Fatalf("entry a usage = %d", loaded[0].UsageCount)
	}
	if loaded[1].UsageCount != 1 || !loaded[1].LastUsedAt.Equal(usedAt) {
		t.Fatalf("entry b = %+v", loaded[1])
	}
	if got, want := filepath.Join(root, "v1", "workspaces", "abc123", "index.json"), filepath.Join(root, "v1", "workspaces", "abc123", "index.json"); got != want {
		t.Fatalf("path mismatch: %q != %q", got, want)
	}
}

func TestFileStoreRejectsUnsafeWorkspaceKey(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	if _, err := store.Load(context.Background(), memory.ScopeWorkspace, "../escape"); err == nil {
		t.Fatal("Load() error = nil, want unsafe workspace key error")
	}
}
