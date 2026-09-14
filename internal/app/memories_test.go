package app

import (
	"context"
	"strconv"
	"testing"
	"time"

	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
)

type memoryInspectorRepo struct {
	workspace []corememory.Entry
	global    []corememory.Entry
}

func (r *memoryInspectorRepo) Load(_ context.Context, scope corememory.Scope, workspaceKey string) ([]corememory.Entry, error) {
	if scope == corememory.ScopeWorkspace && workspaceKey == "ws-1" {
		return append([]corememory.Entry(nil), r.workspace...), nil
	}
	if scope == corememory.ScopeGlobal {
		return append([]corememory.Entry(nil), r.global...), nil
	}
	return nil, nil
}
func (r *memoryInspectorRepo) Replace(context.Context, corememory.Scope, string, []corememory.Entry) error {
	return nil
}
func (r *memoryInspectorRepo) Update(context.Context, corememory.Scope, string, corememory.UpdateFunc) error {
	return nil
}
func (r *memoryInspectorRepo) Forget(_ context.Context, scope corememory.Scope, workspaceKey string, ids []string) (int, error) {
	drop := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		drop[id] = struct{}{}
	}
	removed := 0
	filter := func(entries []corememory.Entry) []corememory.Entry {
		kept := make([]corememory.Entry, 0, len(entries))
		for _, entry := range entries {
			if _, ok := drop[entry.ID]; ok {
				removed++
				continue
			}
			kept = append(kept, entry)
		}
		return kept
	}
	switch {
	case scope == corememory.ScopeWorkspace && workspaceKey == "ws-1":
		r.workspace = filter(r.workspace)
	case scope == corememory.ScopeGlobal:
		r.global = filter(r.global)
	}
	return removed, nil
}
func (r *memoryInspectorRepo) RecordUsage(context.Context, []corememory.UsageRef, time.Time) error {
	return nil
}
func (r *memoryInspectorRepo) ProcessedRevision(context.Context, string) (uint64, bool, error) {
	return 0, false, nil
}
func (r *memoryInspectorRepo) MarkProcessed(context.Context, string, uint64) error { return nil }

func TestMemoriesInspectProjectsWorkspaceAndGlobalWithoutUsageMutation(t *testing.T) {
	repo := &memoryInspectorRepo{
		workspace: []corememory.Entry{{ID: "w1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindRepoFact, Key: "path", Value: "repo root", WorkspaceKey: "ws-1", Confidence: .9, UsageCount: 2}},
		global:    []corememory.Entry{{ID: "g1", Scope: corememory.ScopeGlobal, Kind: corememory.KindPreference, Key: "style", Value: "concise", Confidence: .95, UsageCount: 4}},
	}
	service := NewMemories(repo)
	got, err := service.Inspect(context.Background(), " ws-1 ")
	if err != nil {
		t.Fatal(err)
	}
	if got.WorkspaceKey != "ws-1" || len(got.Workspace) != 1 || len(got.Global) != 1 {
		t.Fatalf("unexpected snapshot: %#v", got)
	}
	if got.Workspace[0].Value != "repo root" || got.Global[0].Value != "concise" {
		t.Fatalf("unexpected entries: %#v", got)
	}
}

func TestMemoriesForgetRemovesWorkspaceEntry(t *testing.T) {
	repo := &memoryInspectorRepo{
		workspace: []corememory.Entry{
			{ID: "w1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindRepoFact, Key: "path", Value: "wrong", WorkspaceKey: "ws-1", Confidence: 1},
			{ID: "w2", Scope: corememory.ScopeWorkspace, Kind: corememory.KindRepoFact, Key: "other", Value: "keep", WorkspaceKey: "ws-1", Confidence: 1},
		},
		global: []corememory.Entry{{ID: "g1", Scope: corememory.ScopeGlobal, Kind: corememory.KindPreference, Key: "style", Value: "concise", Confidence: 1}},
	}
	service := NewMemories(repo)
	got, err := service.Forget(context.Background(), MemoryForgetRequest{
		Scope: MemoryScopeWorkspace, WorkspaceKey: "ws-1", IDs: []string{"w1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Removed != 1 || got.Scope != MemoryScopeWorkspace {
		t.Fatalf("result = %#v, want one workspace removal", got)
	}
	if len(repo.workspace) != 1 || repo.workspace[0].ID != "w2" {
		t.Fatalf("workspace index after forget = %#v, want only w2", repo.workspace)
	}
	if len(repo.global) != 1 || repo.global[0].ID != "g1" {
		t.Fatalf("global index must be untouched by a workspace forget: %#v", repo.global)
	}
}

// TestMemoriesForgetIsFailClosed covers the authority boundary: a blank or
// missing workspace key must not widen into "every workspace".
func TestMemoriesForgetIsFailClosed(t *testing.T) {
	repo := &memoryInspectorRepo{
		workspace: []corememory.Entry{{ID: "w1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindRepoFact, Key: "path", Value: "v", WorkspaceKey: "ws-1", Confidence: 1}},
		global:    []corememory.Entry{{ID: "g1", Scope: corememory.ScopeGlobal, Kind: corememory.KindPreference, Key: "style", Value: "v", Confidence: 1}},
	}
	service := NewMemories(repo)
	for _, tc := range []struct {
		name    string
		request MemoryForgetRequest
	}{
		{name: "workspace without key", request: MemoryForgetRequest{Scope: MemoryScopeWorkspace, IDs: []string{"w1"}}},
		{name: "workspace blank key", request: MemoryForgetRequest{Scope: MemoryScopeWorkspace, WorkspaceKey: "   ", IDs: []string{"w1"}}},
		{name: "workspace combined with global", request: MemoryForgetRequest{Scope: MemoryScopeGlobal, WorkspaceKey: "ws-1", IDs: []string{"g1"}}},
		{name: "unknown scope", request: MemoryForgetRequest{Scope: "everything", IDs: []string{"w1"}}},
		{name: "no ids", request: MemoryForgetRequest{Scope: MemoryScopeWorkspace, WorkspaceKey: "ws-1"}},
		{name: "blank ids only", request: MemoryForgetRequest{Scope: MemoryScopeWorkspace, WorkspaceKey: "ws-1", IDs: []string{"", "  "}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Forget(context.Background(), tc.request); err == nil {
				t.Fatal("expected fail-closed rejection")
			}
			if len(repo.workspace) != 1 || len(repo.global) != 1 {
				t.Fatalf("index mutated by a rejected request: workspace=%#v global=%#v", repo.workspace, repo.global)
			}
		})
	}
}

func TestMemoriesForgetDeduplicatesIDsAndToleratesUnknown(t *testing.T) {
	repo := &memoryInspectorRepo{
		workspace: []corememory.Entry{{ID: "w1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindRepoFact, Key: "path", Value: "v", WorkspaceKey: "ws-1", Confidence: 1}},
	}
	service := NewMemories(repo)
	got, err := service.Forget(context.Background(), MemoryForgetRequest{
		Scope: MemoryScopeWorkspace, WorkspaceKey: " ws-1 ", IDs: []string{"w1", "w1", "missing", " "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Removed != 1 {
		t.Fatalf("removed = %d, want 1 for duplicated/unknown ids", got.Removed)
	}
	if len(repo.workspace) != 0 {
		t.Fatalf("workspace index = %#v, want empty", repo.workspace)
	}
}

func TestMemoriesForgetRejectsOversizedRequest(t *testing.T) {
	ids := make([]string, 0, maxForgetIDs+1)
	for i := 0; i <= maxForgetIDs; i++ {
		ids = append(ids, "id-"+strconv.Itoa(i))
	}
	service := NewMemories(&memoryInspectorRepo{})
	if _, err := service.Forget(context.Background(), MemoryForgetRequest{
		Scope: MemoryScopeWorkspace, WorkspaceKey: "ws-1", IDs: ids,
	}); err == nil {
		t.Fatal("expected oversized forget request rejection")
	}
}
