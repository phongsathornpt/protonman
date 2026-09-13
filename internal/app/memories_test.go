package app

import (
	"context"
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
func (r *memoryInspectorRepo) Replace(context.Context, corememory.Scope, string, []corememory.Entry) error { return nil }
func (r *memoryInspectorRepo) Update(context.Context, corememory.Scope, string, corememory.UpdateFunc) error { return nil }
func (r *memoryInspectorRepo) RecordUsage(context.Context, []corememory.UsageRef, time.Time) error { return nil }
func (r *memoryInspectorRepo) ProcessedRevision(context.Context, string) (uint64, bool, error) { return 0, false, nil }
func (r *memoryInspectorRepo) MarkProcessed(context.Context, string, uint64) error { return nil }

func TestMemoriesInspectProjectsWorkspaceAndGlobalWithoutUsageMutation(t *testing.T) {
	repo := &memoryInspectorRepo{
		workspace: []corememory.Entry{{ID: "w1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindRepoFact, Key: "path", Value: "repo root", WorkspaceKey: "ws-1", Confidence: .9, UsageCount: 2}},
		global: []corememory.Entry{{ID: "g1", Scope: corememory.ScopeGlobal, Kind: corememory.KindPreference, Key: "style", Value: "concise", Confidence: .95, UsageCount: 4}},
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
