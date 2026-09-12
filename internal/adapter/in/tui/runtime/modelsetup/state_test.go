package modelsetup

import (
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestActiveProviderNameFallsBackToOpenCode(t *testing.T) {
	if got := ActiveProviderName([]string{"alpha"}, 9); got != "opencode" {
		t.Fatalf("fallback provider = %q", got)
	}
}

func TestReasoningIndexMatchesDesiredEffort(t *testing.T) {
	choices := []sdk.ReasoningEffort{sdk.ReasoningDefault, sdk.ReasoningLow, sdk.ReasoningHigh}
	if got := ReasoningIndex(choices, sdk.ReasoningHigh); got != 2 {
		t.Fatalf("reasoning index = %d, want 2", got)
	}
}

func TestMoveReasoningWrapsSelection(t *testing.T) {
	choices := []sdk.ReasoningEffort{sdk.ReasoningDefault, sdk.ReasoningLow, sdk.ReasoningHigh}
	index, effort := MoveReasoning(choices, 0, -1)
	if index != 2 || effort != sdk.ReasoningHigh {
		t.Fatalf("move = %d/%q, want 2/high", index, effort)
	}
}
