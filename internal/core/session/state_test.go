package session

import (
	"strings"
	"testing"
	"time"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestFromModelMessagesPreservesCanonicalToolNames(t *testing.T) {
	messages := []sdk.Message{
		{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{{ID: "call-1", Name: "read"}}},
		{Role: sdk.RoleTool, ToolCallID: "call-1", ToolName: "read", Content: "legacy output"},
	}
	persisted := FromModelMessages(messages)
	if len(persisted) != 1 {
		t.Fatalf("persisted messages = %#v", persisted)
	}
	if !strings.Contains(persisted[0].Content, "Historical tool read result") {
		t.Fatalf("canonical tool history missing: %q", persisted[0].Content)
	}
}

func TestMessageIdentitySurvivesSessionConversion(t *testing.T) {
	input := []sdk.Message{{ID: "msg_keep", Role: sdk.RoleUser, Content: "hello"}}
	stored := FromModelMessages(input)
	if len(stored) != 1 || stored[0].ID != "msg_keep" {
		t.Fatalf("stored messages = %+v", stored)
	}
	restored := ToModelMessages(stored)
	if len(restored) != 1 || restored[0].ID != "msg_keep" {
		t.Fatalf("restored messages = %+v", restored)
	}
}

func TestLegacySessionMessagesReceiveStableIdentity(t *testing.T) {
	stored := FromModelMessages([]sdk.Message{{Role: sdk.RoleUser, Content: "legacy"}})
	if len(stored) != 1 || stored[0].ID == "" {
		t.Fatalf("legacy stored message id = %q", stored[0].ID)
	}
	first := ToModelMessages(stored)
	second := ToModelMessages(stored)
	if first[0].ID != stored[0].ID || second[0].ID != stored[0].ID {
		t.Fatalf("legacy identity changed: stored=%q first=%q second=%q", stored[0].ID, first[0].ID, second[0].ID)
	}
}

func TestSessionStatePreservesActiveGoal(t *testing.T) {
	now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	prepared, err := PrepareStateForSave("goal-session", State{
		PermissionMode: "ask",
		ActiveGoal:     "  finish retry recovery  ",
	}, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := NormalizeLoadedState("goal-session", prepared)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveGoal != "finish retry recovery" {
		t.Fatalf("active goal = %q", loaded.ActiveGoal)
	}
}
