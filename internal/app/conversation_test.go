package app

import (
	"errors"
	"strings"
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

func TestPrimaryConversationPolicyCarriesActiveGoal(t *testing.T) {
	got, _, err := primaryConversationPolicy(ConversationSpec{ActiveGoal: "  preserve this goal  "})
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveGoal != "preserve this goal" {
		t.Fatalf("active goal = %q", got.ActiveGoal)
	}
}

func TestSynthesisBatchMessagesUseStructuredResultPayload(t *testing.T) {
	batch := agent.SynthesisBatch{Results: []agent.SynthesisResult{{
		Result: agent.Result{
			AgentID: "agility-1", Profile: agent.ProfileAgility,
			Summary: "found reconnect race", Evidence: []agent.EvidenceRef{{Tool: "read", Target: "session.go"}},
			ChangedTargets: []string{"session.go"},
		},
	}}}
	messages, err := synthesisBatchMessages(batch)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages=%d, want 1", len(messages))
	}
	for _, want := range []string{`"status":"completed"`, `"conclusion":"found reconnect race"`, `"evidence"`, `"changed_targets"`} {
		if !strings.Contains(messages[0].Content, want) {
			t.Fatalf("runtime context missing %q: %s", want, messages[0].Content)
		}
	}
	if strings.Contains(messages[0].Content, `"summary"`) {
		t.Fatalf("runtime context retained legacy summary field: %s", messages[0].Content)
	}
}

func TestSynthesisBatchMessagesExposeBoundedFailureBlocker(t *testing.T) {
	batch := agent.SynthesisBatch{Results: []agent.SynthesisResult{{
		Result: agent.Result{AgentID: "agility-1", Profile: agent.ProfileAgility, Err: errors.New(strings.Repeat("x", runtimeSubagentBlockerBytes*2))},
	}}}
	messages, err := synthesisBatchMessages(batch)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(messages[0].Content, `"status":"failed"`) || !strings.Contains(messages[0].Content, `"blockers"`) {
		t.Fatalf("failure context missing status/blocker: %s", messages[0].Content)
	}
	if len(messages[0].Content) > runtimeSubagentBlockerBytes+1024 {
		t.Fatalf("failure context is unexpectedly large: %d bytes", len(messages[0].Content))
	}
}
