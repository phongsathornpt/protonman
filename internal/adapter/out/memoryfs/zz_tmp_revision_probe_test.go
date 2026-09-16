package memoryfs

import (
	"context"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/memory"
)

// Temporary probe: does usage accounting publish a new revision? The
// memory-request decorator caches retrieval by (workspace, revision, query), and
// retrieval records usage, so a revision bump here would invalidate that cache on
// every subsequent model round.
func TestTmpProbeRecordUsageBumpsRevision(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	entries := []memory.Entry{{
		ID: "mem-1", Scope: memory.ScopeWorkspace, Kind: memory.KindProcedure,
		Key: "test command", Value: "go test ./...", WorkspaceKey: "abc123", Confidence: 1,
		UpdatedAt: time.Now().UTC(),
	}}
	if err := store.Replace(ctx, memory.ScopeWorkspace, "abc123", entries); err != nil {
		t.Fatal(err)
	}
	afterReplace := store.Revision()
	if _, err := store.Load(ctx, memory.ScopeWorkspace, "abc123"); err != nil {
		t.Fatal(err)
	}
	afterLoad := store.Revision()
	if afterLoad != afterReplace {
		t.Fatalf("Load bumped revision: %d -> %d", afterReplace, afterLoad)
	}
	if err := store.RecordUsage(ctx, []memory.UsageRef{{Scope: memory.ScopeWorkspace, WorkspaceKey: "abc123", ID: "mem-1"}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	afterUsage := store.Revision()
	t.Logf("revision replace=%d load=%d usage=%d", afterReplace, afterLoad, afterUsage)
	if afterUsage == afterLoad {
		t.Log("usage accounting did NOT bump revision (cache would hold)")
		return
	}
	t.Log("usage accounting BUMPED revision (decorator cache misses on every round)")
}
