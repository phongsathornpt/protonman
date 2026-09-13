package memory

import (
	"context"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
)

func TestRetrieverDeduplicatesEquivalentWorkspaceAndGlobalMemory(t *testing.T) {
	now := time.Date(2026, 9, 13, 6, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		workspace: []corememory.Entry{{
			ID:           "workspace-style",
			Scope:        corememory.ScopeWorkspace,
			Kind:         corememory.KindPreference,
			Key:          "verification workflow",
			Value:        "run workspace-specific verification",
			Keywords:     []string{"verification"},
			WorkspaceKey: "ws",
			Confidence:   0.8,
			UpdatedAt:    now.Add(-time.Hour),
		}},
		global: []corememory.Entry{{
			ID:         "global-style",
			Scope:      corememory.ScopeGlobal,
			Kind:       corememory.KindPreference,
			Key:        " Verification Workflow ",
			Value:      "run generic verification",
			Keywords:   []string{"verification"},
			Confidence: 1,
			UpdatedAt:  now,
		}},
	}

	entries, err := NewRetriever(repo, runtimepolicy.DurableMemory()).Retrieve(context.Background(), Query{
		WorkspaceKey: "ws",
		Text:         "verification workflow",
		Now:          now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want one deduplicated memory", entries)
	}
	if entries[0].ID != "workspace-style" {
		t.Fatalf("selected entry = %q, want workspace override", entries[0].ID)
	}
	if len(repo.used) != 1 || repo.used[0].ID != "workspace-style" {
		t.Fatalf("usage refs = %+v, want only selected workspace memory", repo.used)
	}
}

func TestMemoryIdentityNormalizesKindAndKeyIndependently(t *testing.T) {
	got := memoryIdentity(corememory.Kind(" PREFERENCE "), "  Verification Workflow  ")
	if got != "preference|verification workflow" {
		t.Fatalf("memoryIdentity() = %q", got)
	}
}
