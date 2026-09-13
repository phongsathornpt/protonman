package memory

import (
	"testing"
	"time"

	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
)

func TestMergeEntriesAllowsNewerNearConfidenceReplacement(t *testing.T) {
	oldTime := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(24 * time.Hour)
	existing := []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "build command", Value: "go test ./old/...", Confidence: 0.95,
		CreatedAt: oldTime, UpdatedAt: oldTime,
	}}
	incoming := []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "build command", Value: "go test ./...", Confidence: 0.90,
		CreatedAt: newTime, UpdatedAt: newTime,
	}}
	merged := mergeEntries(existing, incoming, 10)
	if len(merged) != 1 {
		t.Fatalf("merged = %+v", merged)
	}
	if merged[0].Value != "go test ./..." || merged[0].Confidence != 0.90 {
		t.Fatalf("newer near-confidence memory was not adopted: %+v", merged[0])
	}
}

func TestMergeEntriesRejectsMuchLowerConfidenceReplacement(t *testing.T) {
	oldTime := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(24 * time.Hour)
	existing := []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "build command", Value: "go test ./old/...", Confidence: 0.95,
		CreatedAt: oldTime, UpdatedAt: oldTime,
	}}
	incoming := []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "build command", Value: "rm -rf build", Confidence: 0.70,
		CreatedAt: newTime, UpdatedAt: newTime,
	}}
	merged := mergeEntries(existing, incoming, 10)
	if len(merged) != 1 {
		t.Fatalf("merged = %+v", merged)
	}
	if merged[0].Value != "go test ./old/..." || merged[0].Confidence != 0.95 {
		t.Fatalf("low-confidence replacement was accepted: %+v", merged[0])
	}
	if !merged[0].UpdatedAt.Equal(newTime) {
		t.Fatalf("evidence freshness was not retained: %+v", merged[0])
	}
}
