package memory

import (
	"context"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
)

func TestRetrieverIgnoresStopWordOnlyOverlap(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{workspace: []corememory.Entry{{
		ID: "irrelevant", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "deployment workflow", Value: "This is the process to deploy the service in production",
		WorkspaceKey: "ws", Confidence: 1, UpdatedAt: now,
	}}}
	entries, err := NewRetriever(repo, runtimepolicy.DurableMemory()).Retrieve(context.Background(), Query{
		WorkspaceKey: "ws",
		Text:         "Where is the main entry point?",
		Now:          now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %+v, want no stop-word-only match", entries)
	}
	if len(repo.used) != 0 {
		t.Fatalf("usage recorded for irrelevant memory: %+v", repo.used)
	}
}

func TestRetrieverRequiresAnchoredKeyOrKeywordRelevance(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{workspace: []corememory.Entry{{
		ID: "value-only", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "release workflow", Value: "Run protonman verification tests before merging",
		WorkspaceKey: "ws", Confidence: 1, UsageCount: 1000, UpdatedAt: now,
	}}}
	entries, err := NewRetriever(repo, runtimepolicy.DurableMemory()).Retrieve(context.Background(), Query{
		WorkspaceKey: "ws",
		Text:         "protonman verification tests",
		Now:          now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %+v, want weak value-only match rejected", entries)
	}
	if len(repo.used) != 0 {
		t.Fatalf("usage recorded for weak value-only match: %+v", repo.used)
	}
}

func TestRetrieverAcceptsKeywordAnchorBeforeBonuses(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{workspace: []corememory.Entry{{
		ID: "anchored", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "release workflow", Value: "Run focused checks before merging", Keywords: []string{"verification"},
		WorkspaceKey: "ws", Confidence: 0.8, UpdatedAt: now,
	}}}
	entries, err := NewRetriever(repo, runtimepolicy.DurableMemory()).Retrieve(context.Background(), Query{
		WorkspaceKey: "ws",
		Text:         "verification",
		Now:          now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "anchored" {
		t.Fatalf("entries = %+v, want anchored memory", entries)
	}
}
