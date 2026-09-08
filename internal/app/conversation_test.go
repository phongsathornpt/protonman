package app

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestPrimaryConversationProfileDoesNotBecomeSubagentRole(t *testing.T) {
	prompt, _, err := primaryConversationPolicy(ConversationSpec{AgentProfile: string(agent.ProfileStrength)})
	if err != nil {
		t.Fatal(err)
	}
	if prompt.Profile != string(agent.ProfileStrength) {
		t.Fatalf("profile = %q", prompt.Profile)
	}
	if prompt.Role != "" {
		t.Fatalf("primary conversation role = %q, want empty", prompt.Role)
	}
}

func TestPrimaryConversationRejectsUnknownProfile(t *testing.T) {
	if _, _, err := primaryConversationPolicy(ConversationSpec{AgentProfile: "unknown"}); err == nil {
		t.Fatal("expected invalid profile error")
	}
}
