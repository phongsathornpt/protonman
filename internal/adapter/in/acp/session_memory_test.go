package acp

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/app"
)

func TestProjectMemoryEntries(t *testing.T) {
	input := []app.MemoryEntry{{
		ID: "m1", Scope: "workspace", Kind: "repo_fact", Key: "path", Value: "repo root", Confidence: .9, UsageCount: 3,
	}}
	got := projectMemoryEntries(input)
	if len(got) != 1 {
		t.Fatalf("entries = %#v", got)
	}
	if got[0].ID != "m1" || got[0].Kind != "repo_fact" || got[0].UsageCount != 3 {
		t.Fatalf("entry = %#v", got[0])
	}
	input[0].Value = "mutated"
	if got[0].Value != "repo root" {
		t.Fatalf("projection aliased input: %#v", got[0])
	}
}
