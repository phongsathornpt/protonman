package reasoningpolicy

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

func TestChoicesKeepsAutoFirst(t *testing.T) {
	profile := modelprofile.Resolved{}
	profile.Reasoning.Levels = []domain.ReasoningEffort{domain.ReasoningLow, domain.ReasoningHigh}
	got := Choices(profile)
	want := []domain.ReasoningEffort{domain.ReasoningDefault, domain.ReasoningLow, domain.ReasoningHigh}
	if len(got) != len(want) {
		t.Fatalf("Choices() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Choices() = %v, want %v", got, want)
		}
	}
}
