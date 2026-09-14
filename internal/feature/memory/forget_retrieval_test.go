package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/memoryfs"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
	"github.com/phongsathornpt/protonman/internal/core/modelclient"
	"github.com/phongsathornpt/protonman/internal/core/session"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func memoryForgetTestRequest() sdk.Request {
	return sdk.Request{Messages: []sdk.Message{
		{ID: "system", Role: sdk.RoleSystem, Content: "stable system prompt"},
		{ID: "user-1", Role: sdk.RoleUser, Content: "please run test verification"},
	}}
}

// TestForgetStopsFutureRetrieval is the user-facing contract for correction: a
// remembered fact that turns out to be wrong must stop reaching future turns
// once it is forgotten, including through an already-built decorated model.
func TestForgetStopsFutureRetrieval(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{workspace: []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "verification tests", Value: "orphaned-memory-value", Keywords: []string{"test"},
		WorkspaceKey: "ws", Confidence: 1, UpdatedAt: now,
	}}}
	base := &captureModel{}
	factory := NewModelFactory(captureFactory{model: base}, repo, "ws", runtimepolicy.DurableMemory())
	model := factory.Build(modelclient.Request{ModelID: "test-model"})

	stream, err := model.Stream(context.Background(), memoryForgetTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	if len(base.requests) != 1 || !strings.Contains(base.requests[0].Messages[1].Content, "orphaned-memory-value") {
		t.Fatalf("memory was not retrieved before forget: %+v", base.requests)
	}

	removed, err := repo.Forget(context.Background(), corememory.ScopeWorkspace, "ws", []string{"mem-1"})
	if err != nil || removed != 1 {
		t.Fatalf("forget removed = %d, err = %v", removed, err)
	}

	stream, err = model.Stream(context.Background(), memoryForgetTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	if len(base.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(base.requests))
	}
	if strings.Contains(base.requests[1].Messages[1].Content, "orphaned-memory-value") {
		t.Fatalf("forgotten memory still reached the model: %q", base.requests[1].Messages[1].Content)
	}
	if base.requests[1].Messages[1].Content != "please run test verification" {
		t.Fatalf("current user message = %q, want undecorated", base.requests[1].Messages[1].Content)
	}
}

// TestExtractionCannotResurrectForgottenMemory is the end-to-end durability
// regression for forget. It runs the real Extractor against the real FileStore:
// extraction derives a stable ID deterministically and merges by ID, so a plain
// delete would be silently undone by the next extraction pass over the same
// source session. The workspace-scope tombstone must prevent that.
func TestExtractionCannotResurrectForgottenMemory(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	store, err := memoryfs.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const output = `{"memories":[{"scope":"workspace","kind":"procedure","key":"verification command","value":"Run go test ./... before finishing","keywords":["go test"],"confidence":0.95,"message_ids":["m1"]}]}`

	run := func(revision uint64) {
		t.Helper()
		state := session.State{
			SessionID: "previous", Revision: revision, WorkspaceKey: "ws", UpdatedAt: now.Add(-time.Hour),
			Messages: []session.Message{{ID: "m1", Role: sdk.RoleUser, Content: "Run go test ./... before finishing."}},
		}
		sessions := &extractionSessionRepo{
			states:    map[string]session.State{"previous": state},
			summaries: []session.Summary{{ID: "previous", WorkspaceKey: "ws", UpdatedAt: state.UpdatedAt}},
		}
		extractor := NewExtractor(sessions, store, &extractionModel{output: output}, "current", "ws", runtimepolicy.DurableMemory())
		extractor.now = func() time.Time { return now }
		if err := extractor.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}

	run(3)
	entries, err := store.Load(ctx, corememory.ScopeWorkspace, "ws")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("extraction seeded %d entries, want 1", len(entries))
	}
	forgottenID := entries[0].ID

	if removed, err := store.Forget(ctx, corememory.ScopeWorkspace, "ws", []string{forgottenID}); err != nil || removed != 1 {
		t.Fatalf("Forget() = %d, %v; want 1, nil", removed, err)
	}

	// A later revision of the same session re-runs extraction and re-derives the
	// same stable ID from the transcript.
	run(4)
	remaining, err := store.Load(ctx, corememory.ScopeWorkspace, "ws")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range remaining {
		if entry.ID == forgottenID {
			t.Fatalf("extraction resurrected forgotten memory %q: %+v", forgottenID, entry)
		}
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining entries = %+v, want none", remaining)
	}
}

// TestForgetRetrievalRechecksStoreNotCache documents that a forgotten entry
// stops being served on the next distinct query even while the decorator is
// reused across rounds. The decorator caches by query text, so a repeated
// identical query within a round legitimately reuses the earlier context.
func TestForgetRetrievalRechecksStoreNotCache(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{workspace: []corememory.Entry{
		{ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure, Key: "verification tests", Value: "orphaned-memory-value", Keywords: []string{"test"}, WorkspaceKey: "ws", Confidence: 1, UpdatedAt: now},
		{ID: "mem-2", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure, Key: "other procedure", Value: "kept-memory-value", Keywords: []string{"test"}, WorkspaceKey: "ws", Confidence: 1, UpdatedAt: now},
	}}
	if _, err := repo.Forget(context.Background(), corememory.ScopeWorkspace, "ws", []string{"mem-1"}); err != nil {
		t.Fatal(err)
	}
	base := &captureModel{}
	model := NewModelFactory(captureFactory{model: base}, repo, "ws", runtimepolicy.DurableMemory()).Build(modelclient.Request{ModelID: "test-model"})
	stream, err := model.Stream(context.Background(), memoryForgetTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	got := base.requests[0].Messages[1].Content
	if strings.Contains(got, "orphaned-memory-value") {
		t.Fatalf("forgotten entry retrieved from a fresh store read: %q", got)
	}
	if !strings.Contains(got, "kept-memory-value") {
		t.Fatalf("unrelated memory was lost: %q", got)
	}
}

// TestExtractionCannotResurrectForgottenGlobalMemory covers the global scope.
// The workspace branch of mergeCandidates filters promoted identities before
// merging, while the global branch merges directly, so the tombstone filter in
// the repository write path is the only thing preventing resurrection here.
func TestExtractionCannotResurrectForgottenGlobalMemory(t *testing.T) {
	now := time.Date(2026, 9, 13, 4, 0, 0, 0, time.UTC)
	store, err := memoryfs.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// An explicitly stated global preference is promoted without needing the
	// multi-session threshold.
	const output = `{"memories":[{"scope":"global","kind":"preference","key":"commit style","value":"Always sign commits with SSH","confidence":0.95,"message_ids":["m1"],"explicit":true}]}`

	run := func(revision uint64) {
		t.Helper()
		state := session.State{
			SessionID: "previous", Revision: revision, WorkspaceKey: "ws", UpdatedAt: now.Add(-time.Hour),
			Messages: []session.Message{{ID: "m1", Role: sdk.RoleUser, Content: "Always sign commits with SSH."}},
		}
		sessions := &extractionSessionRepo{
			states:    map[string]session.State{"previous": state},
			summaries: []session.Summary{{ID: "previous", WorkspaceKey: "ws", UpdatedAt: state.UpdatedAt}},
		}
		extractor := NewExtractor(sessions, store, &extractionModel{output: output}, "current", "ws", runtimepolicy.DurableMemory())
		extractor.now = func() time.Time { return now }
		if err := extractor.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}

	run(3)
	entries, err := store.Load(ctx, corememory.ScopeGlobal, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("extraction seeded %d global entries, want 1", len(entries))
	}
	forgottenID := entries[0].ID

	if removed, err := store.Forget(ctx, corememory.ScopeGlobal, "", []string{forgottenID}); err != nil || removed != 1 {
		t.Fatalf("Forget() = %d, %v; want 1, nil", removed, err)
	}

	run(4)
	remaining, err := store.Load(ctx, corememory.ScopeGlobal, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range remaining {
		if entry.ID == forgottenID {
			t.Fatalf("extraction resurrected forgotten global memory %q: %+v", forgottenID, entry)
		}
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining global entries = %+v, want none", remaining)
	}
}
