package memoryfs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/memory"
)

func TestFileStoreForgetRemovesOnlyRequestedEntries(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	ctx := context.Background()
	entries := []memory.Entry{
		{ID: "a", Scope: memory.ScopeWorkspace, Kind: memory.KindRepoFact, Key: "a", Value: "wrong", WorkspaceKey: "abc123", Confidence: 1},
		{ID: "b", Scope: memory.ScopeWorkspace, Kind: memory.KindProcedure, Key: "b", Value: "keep", WorkspaceKey: "abc123", Confidence: 1},
	}
	if err := store.Replace(ctx, memory.ScopeWorkspace, "abc123", entries); err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(ctx, memory.ScopeGlobal, "", []memory.Entry{
		{ID: "g1", Scope: memory.ScopeGlobal, Kind: memory.KindPreference, Key: "style", Value: "keep", Confidence: 1},
	}); err != nil {
		t.Fatal(err)
	}

	removed, err := store.Forget(ctx, memory.ScopeWorkspace, "abc123", []string{"a", "a", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}

	workspaceEntries, err := store.Load(ctx, memory.ScopeWorkspace, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaceEntries) != 1 || workspaceEntries[0].ID != "b" {
		t.Fatalf("workspace entries = %+v, want only b", workspaceEntries)
	}

	globalEntries, err := store.Load(ctx, memory.ScopeGlobal, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(globalEntries) != 1 || globalEntries[0].ID != "g1" {
		t.Fatalf("global scope must be untouched: %+v", globalEntries)
	}

	// The removed entry must not come back on a fresh read from disk.
	reopened, _ := NewFileStore(store.root)
	reloaded, err := reopened.Load(ctx, memory.ScopeWorkspace, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded) != 1 || reloaded[0].ID != "b" {
		t.Fatalf("reloaded entries = %+v, want durable removal", reloaded)
	}
}

func TestFileStoreForgetIsNoOpForUnknownIDsAndLeavesNoTempFiles(t *testing.T) {
	root := t.TempDir()
	store, _ := NewFileStore(root)
	ctx := context.Background()
	if err := store.Replace(ctx, memory.ScopeGlobal, "", []memory.Entry{
		{ID: "g1", Scope: memory.ScopeGlobal, Kind: memory.KindPreference, Key: "style", Value: "keep", Confidence: 1},
	}); err != nil {
		t.Fatal(err)
	}

	removed, err := store.Forget(ctx, memory.ScopeGlobal, "", []string{"nope"})
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}
	entries, err := store.Load(ctx, memory.ScopeGlobal, "")
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %+v err = %v", entries, err)
	}

	// A no-op forget must not leave a temporary index or a stale lock behind.
	entriesOnDisk, err := os.ReadDir(filepath.Join(root, "v1", "global"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entriesOnDisk {
		if filepath.Ext(entry.Name()) == ".tmp" || entry.Name() == "index.json.lock" {
			t.Fatalf("unexpected residue after no-op forget: %s", entry.Name())
		}
	}
}

func TestFileStoreForgetWithNoIDsDoesNotTouchIndex(t *testing.T) {
	root := t.TempDir()
	store, _ := NewFileStore(root)
	ctx := context.Background()
	if err := store.Replace(ctx, memory.ScopeGlobal, "", []memory.Entry{
		{ID: "g1", Scope: memory.ScopeGlobal, Kind: memory.KindPreference, Key: "style", Value: "keep", Confidence: 1},
	}); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, "v1", "global", "index.json")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}

	removed, err := store.Forget(ctx, memory.ScopeGlobal, "", nil)
	if err != nil || removed != 0 {
		t.Fatalf("removed = %d err = %v", removed, err)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("empty forget rewrote the index instead of short-circuiting")
	}
}

func TestFileStoreForgetRejectsInvalidScopeBoundary(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	ctx := context.Background()
	if _, err := store.Forget(ctx, memory.ScopeGlobal, "abc123", []string{"a"}); err == nil {
		t.Fatal("expected global scope with a workspace key to be rejected")
	}
	if _, err := store.Forget(ctx, memory.ScopeWorkspace, "", []string{"a"}); err == nil {
		t.Fatal("expected workspace scope without a key to be rejected")
	}
}

// TestFileStoreForgetSerializesWithConcurrentWrites verifies that forgetting
// shares the per-scope write lock with Replace/Update, so concurrent writers
// never observe a partial delete or corrupt the index.
func TestFileStoreForgetSerializesWithConcurrentWrites(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	ctx := context.Background()
	entries := make([]memory.Entry, 0, 16)
	for i := 0; i < 16; i++ {
		entries = append(entries, memory.Entry{
			ID: "id-" + strconv.Itoa(i), Scope: memory.ScopeWorkspace, Kind: memory.KindProcedure,
			Key: "key-" + strconv.Itoa(i), Value: "value", WorkspaceKey: "abc123", Confidence: 1,
		})
	}
	if err := store.Replace(ctx, memory.ScopeWorkspace, "abc123", entries); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = store.Forget(ctx, memory.ScopeWorkspace, "abc123", []string{"id-" + strconv.Itoa(i)})
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = store.Update(ctx, memory.ScopeWorkspace, "abc123", func(current []memory.Entry) ([]memory.Entry, error) {
				return current, nil
			})
		}()
	}
	wg.Wait()

	// Every load must remain structurally valid; the surviving set is a subset.
	remaining, err := store.Load(ctx, memory.ScopeWorkspace, "abc123")
	if err != nil {
		t.Fatalf("index unreadable after concurrent forgets: %v", err)
	}
	if len(remaining) >= len(entries) {
		t.Fatalf("remaining = %d, want fewer than %d", len(remaining), len(entries))
	}
	for _, entry := range remaining {
		if entry.Scope != memory.ScopeWorkspace || entry.WorkspaceKey != "abc123" {
			t.Fatalf("corrupted entry after concurrent writes: %+v", entry)
		}
	}
	// The scope lock must be fully released: no stale lock directory or temp file.
	residue, err := os.ReadDir(filepath.Join(store.root, "v1", "workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range residue {
		if filepath.Ext(entry.Name()) == ".lock" || filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("lock or temp file leaked: %s", entry.Name())
		}
	}
}

// TestFileStoreForgetSurvivesLaterUpdateMerge is the regression that made
// forgetting durable. Extraction derives an entry's stable ID deterministically
// and merges by ID, so a plain delete would be silently undone by the next
// extraction pass. A tombstone must suppress the re-created entry.
func TestFileStoreForgetSurvivesLaterUpdateMerge(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	ctx := context.Background()
	entry := memory.Entry{
		ID: "mem_stable", Scope: memory.ScopeWorkspace, Kind: memory.KindProcedure,
		Key: "verification tests", Value: "wrong", WorkspaceKey: "abc123", Confidence: 1,
	}
	if err := store.Replace(ctx, memory.ScopeWorkspace, "abc123", []memory.Entry{entry}); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.Forget(ctx, memory.ScopeWorkspace, "abc123", []string{"mem_stable"}); err != nil || removed != 1 {
		t.Fatalf("Forget() = %d, %v; want 1, nil", removed, err)
	}

	// Simulate the extraction pass re-deriving the same stable ID.
	recreated := entry
	recreated.Value = "resurrected"
	if err := store.Update(ctx, memory.ScopeWorkspace, "abc123", func(current []memory.Entry) ([]memory.Entry, error) {
		return append(current, recreated), nil
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load(ctx, memory.ScopeWorkspace, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("forgotten memory was resurrected by extraction merge: %+v", loaded)
	}
}

// TestFileStoreForgetSurvivesUsageAccounting covers the other writer. Usage
// accounting rewrites the index and must not drop the tombstone either.
func TestFileStoreForgetSurvivesUsageAccounting(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	ctx := context.Background()
	entry := memory.Entry{
		ID: "mem_stable", Scope: memory.ScopeWorkspace, Kind: memory.KindProcedure,
		Key: "verification tests", Value: "wrong", WorkspaceKey: "abc123", Confidence: 1,
	}
	if err := store.Replace(ctx, memory.ScopeWorkspace, "abc123", []memory.Entry{entry}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Forget(ctx, memory.ScopeWorkspace, "abc123", []string{"mem_stable"}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordUsage(ctx, []memory.UsageRef{
		{Scope: memory.ScopeWorkspace, WorkspaceKey: "abc123", ID: "mem_stable"},
	}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, memory.ScopeWorkspace, "abc123", func(current []memory.Entry) ([]memory.Entry, error) {
		return append(current, entry), nil
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx, memory.ScopeWorkspace, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("tombstone lost across usage accounting: %+v", loaded)
	}
}

// TestFileStoreForgetTombstonePersistsAcrossReopen proves the suppression
// survives a process restart, since it is stored in the index file itself.
func TestFileStoreForgetTombstonePersistsAcrossReopen(t *testing.T) {
	root := t.TempDir()
	store, _ := NewFileStore(root)
	ctx := context.Background()
	entry := memory.Entry{
		ID: "mem_stable", Scope: memory.ScopeWorkspace, Kind: memory.KindProcedure,
		Key: "verification tests", Value: "wrong", WorkspaceKey: "abc123", Confidence: 1,
	}
	if err := store.Replace(ctx, memory.ScopeWorkspace, "abc123", []memory.Entry{entry}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Forget(ctx, memory.ScopeWorkspace, "abc123", []string{"mem_stable"}); err != nil {
		t.Fatal(err)
	}

	reopened, _ := NewFileStore(root)
	if err := reopened.Update(ctx, memory.ScopeWorkspace, "abc123", func(current []memory.Entry) ([]memory.Entry, error) {
		return append(current, entry), nil
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load(ctx, memory.ScopeWorkspace, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("tombstone did not persist across reopen: %+v", loaded)
	}
}

// TestFileStoreForgetTombstoneIsWrittenToIndex asserts the raw on-disk index,
// not just the filtered read. It proves the suppression is actually persisted
// (so it survives a process restart) and that a later merge cannot write the
// forgotten entry back into the index.
func TestFileStoreForgetTombstoneIsWrittenToIndex(t *testing.T) {
	root := t.TempDir()
	store, _ := NewFileStore(root)
	ctx := context.Background()
	entry := memory.Entry{
		ID: "mem_stable", Scope: memory.ScopeWorkspace, Kind: memory.KindProcedure,
		Key: "verification tests", Value: "wrong", WorkspaceKey: "abc123", Confidence: 1,
	}
	if err := store.Replace(ctx, memory.ScopeWorkspace, "abc123", []memory.Entry{entry}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Forget(ctx, memory.ScopeWorkspace, "abc123", []string{"mem_stable"}); err != nil {
		t.Fatal(err)
	}
	// A later extraction merge re-derives the same stable ID.
	if err := store.Update(ctx, memory.ScopeWorkspace, "abc123", func(current []memory.Entry) ([]memory.Entry, error) {
		return append(current, entry), nil
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(root, "v1", "workspaces", "abc123", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index indexFile
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Entries) != 0 {
		t.Fatalf("forgotten entry was written back to the index: %+v", index.Entries)
	}
	found := false
	for _, id := range index.Forgotten {
		if id == "mem_stable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tombstone missing from on-disk index: %+v", index.Forgotten)
	}
}

// TestMergeForgottenRetainsOlderTombstones verifies that an old correction is
// not evicted and later recreated by extraction.
func TestMergeForgottenRetainsOlderTombstones(t *testing.T) {
	existing := []string{"old-000000"}
	got := mergeForgotten(existing, map[string]struct{}{"newest": {}})
	if len(got) != 2 {
		t.Fatalf("tombstone set = %d, want 2", len(got))
	}
	for _, want := range []string{"old-000000", "newest"} {
		found := false
		for _, id := range got {
			if id == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("tombstone %q was evicted: %v", want, got)
		}
	}
}
