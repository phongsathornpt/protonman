package reasoningpolicy

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestChoicesKeepsAutoFirst(t *testing.T) {
	profile := modelprofile.Resolved{}
	profile.Reasoning.Levels = []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningHigh}
	got := Choices(profile)
	want := []sdk.ReasoningEffort{sdk.ReasoningDefault, sdk.ReasoningLow, sdk.ReasoningHigh}
	if len(got) != len(want) {
		t.Fatalf("Choices() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Choices() = %v, want %v", got, want)
		}
	}
}
