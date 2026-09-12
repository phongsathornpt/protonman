package model

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
)

func TestCloneResolvedModelProfileDiscardsLegacyPromptHints(t *testing.T) {
	profile := modelprofile.Resolved{
		AgentPolicy: modelprofile.AgentPolicy{PromptHints: []string{"model-specific prose"}},
	}

	got := cloneResolvedModelProfile(profile)
	if len(got.AgentPolicy.PromptHints) != 0 {
		t.Fatalf("legacy prompt hints survived model profile clone: %v", got.AgentPolicy.PromptHints)
	}
}
