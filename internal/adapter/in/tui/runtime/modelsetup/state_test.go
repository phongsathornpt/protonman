package modelsetup

import (
	"testing"

	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

func TestActiveProviderNameFallsBackToOpenCode(t *testing.T) {
	if got := ActiveProviderName([]string{"alpha"}, 9); got != "opencode" {
		t.Fatalf("fallback provider = %q", got)
	}
}

func TestReasoningIndexMatchesDesiredEffort(t *testing.T) {
	choices := []domain.ReasoningEffort{domain.ReasoningDefault, domain.ReasoningLow, domain.ReasoningHigh}
	if got := ReasoningIndex(choices, domain.ReasoningHigh); got != 2 {
		t.Fatalf("reasoning index = %d, want 2", got)
	}
}

func TestMoveReasoningWrapsSelection(t *testing.T) {
	choices := []domain.ReasoningEffort{domain.ReasoningDefault, domain.ReasoningLow, domain.ReasoningHigh}
	index, effort := MoveReasoning(choices, 0, -1)
	if index != 2 || effort != domain.ReasoningHigh {
		t.Fatalf("move = %d/%q, want 2/high", index, effort)
	}
}
