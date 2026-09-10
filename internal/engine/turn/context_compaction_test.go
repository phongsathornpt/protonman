package turn

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestEffectiveInputBudgetUsesTightestPublishedLimit(t *testing.T) {
	limits := sdk.TokenLimits{ContextWindow: 10_000, MaxInputTokens: 9_000, MaxOutputTokens: 2_000}
	if got := effectiveInputBudget(limits, 2_000); got != 8_000 {
		t.Fatalf("budget = %d, want 8000", got)
	}
}

func TestCompactRequestPreservesLatestTurn(t *testing.T) {
	messages := []sdk.Message{{Role: sdk.RoleSystem, Content: "system"}}
	for i := 0; i < 24; i++ {
		messages = append(messages,
			sdk.Message{Role: sdk.RoleUser, Content: strings.Repeat("old-user-", 40)},
			sdk.Message{Role: sdk.RoleAssistant, Content: strings.Repeat("old-assistant-", 40)},
		)
	}
	messages = append(messages, sdk.Message{Role: sdk.RoleUser, Content: "CURRENT-USER-TURN"})
	request := sdk.Request{Messages: messages}
	policy := modelprofile.CompactionPolicy{
		SoftThresholdRatio: 0.30, MediumThresholdRatio: 0.50,
		AggressiveThresholdRatio: 0.70, EmergencyThresholdRatio: 0.90,
		TargetRatio: 0.25, MinRecentMessages: 4,
	}
	got, decision, err := compactRequestToModelBudget(request, sdk.TokenLimits{MaxInputTokens: 20_000}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Required() {
		t.Fatalf("decision = %+v, want compaction", decision)
	}
	if len(got.Messages) >= len(messages) {
		t.Fatalf("message count = %d, want less than %d", len(got.Messages), len(messages))
	}
	if got.Messages[0].Role != sdk.RoleSystem || got.Messages[0].Content != "system" {
		t.Fatalf("leading system message not preserved: %+v", got.Messages[0])
	}
	if got.Messages[len(got.Messages)-1].Content != "CURRENT-USER-TURN" {
		t.Fatalf("latest user turn not preserved: %+v", got.Messages[len(got.Messages)-1])
	}
}
