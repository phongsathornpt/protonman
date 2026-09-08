package pane

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestReasoningChoicesWithKnownLevels(t *testing.T) {
	profile := modelprofile.Resolved{
		Reasoning: modelprofile.Reasoning{
			Support: modelprofile.SupportYes,
			Levels:  []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningMedium, sdk.ReasoningHigh},
		},
	}
	choices := ReasoningChoices(profile)
	if len(choices) != 4 {
		t.Fatalf("len(choices) = %d, want 4", len(choices))
	}
	if choices[0] != sdk.ReasoningDefault || choices[1] != sdk.ReasoningLow || choices[2] != sdk.ReasoningMedium || choices[3] != sdk.ReasoningHigh {
		t.Fatalf("unexpected choices: %v", choices)
	}
}

func TestReasoningChoicesWithUnknownSupportProvidesStandardLevels(t *testing.T) {
	profile := modelprofile.Resolved{
		Reasoning: modelprofile.Reasoning{
			Support: modelprofile.SupportUnknown,
			Levels:  nil,
		},
	}
	choices := ReasoningChoices(profile)
	if len(choices) != 4 {
		t.Fatalf("len(choices) = %d, want 4 standard levels for SupportUnknown", len(choices))
	}
	if choices[0] != sdk.ReasoningDefault || choices[1] != sdk.ReasoningLow || choices[2] != sdk.ReasoningMedium || choices[3] != sdk.ReasoningHigh {
		t.Fatalf("unexpected choices for SupportUnknown: %v", choices)
	}
}

func TestReasoningChoicesWithNoSupportReturnsDefaultOnly(t *testing.T) {
	profile := modelprofile.Resolved{
		Reasoning: modelprofile.Reasoning{
			Support: modelprofile.SupportNo,
		},
	}
	choices := ReasoningChoices(profile)
	if len(choices) != 1 || choices[0] != sdk.ReasoningDefault {
		t.Fatalf("choices = %v, want [auto] for SupportNo", choices)
	}
}

func TestReasoningRowsShowsEffectiveResolutionHints(t *testing.T) {
	snapshotNoSupport := ReasoningSnapshot{
		Height:    24,
		Index:     0,
		ModelName: "gpt-4o",
		Current:   sdk.ReasoningDefault,
		ModelProfile: modelprofile.Resolved{
			Reasoning: modelprofile.Reasoning{
				Support: modelprofile.SupportNo,
			},
		},
	}
	rows := ReasoningRows(snapshotNoSupport)
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "extended reasoning not supported") {
		t.Fatalf("rows = %q, want notice that extended reasoning is not supported", joined)
	}

	snapshotWithDefault := ReasoningSnapshot{
		Height:    24,
		Index:     0,
		ModelName: "o3-mini",
		Current:   sdk.ReasoningDefault,
		ModelProfile: modelprofile.Resolved{
			Reasoning: modelprofile.Reasoning{
				Support: modelprofile.SupportYes,
				Default: sdk.ReasoningMedium,
				Levels:  []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningMedium, sdk.ReasoningHigh},
			},
		},
	}
	rowsDefault := ReasoningRows(snapshotWithDefault)
	joinedDefault := strings.Join(rowsDefault, "\n")
	if !strings.Contains(joinedDefault, "resolves to: medium") {
		t.Fatalf("rows = %q, want notice that auto resolves to medium", joinedDefault)
	}
}
