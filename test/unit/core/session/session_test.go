package session_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/core/session"
	"github.com/projectTHORN/proton/internal/core/tool"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func TestToModelMessagesAndFromModelMessages(t *testing.T) {
	input := []sdk.Message{
		{Role: sdk.RoleUser, Content: "hello"},
		{Role: sdk.RoleAssistant, Content: "world"},
	}
	stored := session.FromModelMessages(input)
	if len(stored) != 2 {
		t.Fatalf("FromModelMessages returned %d messages, want 2", len(stored))
	}
	if stored[0].Role != sdk.RoleUser || stored[0].Content != "hello" {
		t.Errorf("stored[0] = %+v", stored[0])
	}
	if stored[1].Role != sdk.RoleAssistant || stored[1].Content != "world" {
		t.Errorf("stored[1] = %+v", stored[1])
	}

	restored := session.ToModelMessages(stored)
	if len(restored) != 2 {
		t.Fatalf("ToModelMessages returned %d messages, want 2", len(restored))
	}
	if restored[0].Role != sdk.RoleUser || restored[0].Content != "hello" {
		t.Errorf("restored[0] = %+v", restored[0])
	}
	if restored[1].Role != sdk.RoleAssistant || restored[1].Content != "world" {
		t.Errorf("restored[1] = %+v", restored[1])
	}
}

func TestManagedSystemPromptFiltering(t *testing.T) {
	managedPrompts := []string{
		`<proton-system-prompt version="5">instructions</proton-system-prompt>`,
		"You are Proton, an autonomous coding agent operating inside a real workspace.",
		"You are an Explorer subagent in Proton.",
		"You are a Code Reviewer subagent in Proton.",
		"You are a Worker subagent in Proton.",
		"You are Proton in POW Mode",
		"You are Proton in DEX Mode",
		"You are Proton in INT Mode",
	}

	for _, p := range managedPrompts {
		stored := session.FromModelMessages([]sdk.Message{{Role: sdk.RoleSystem, Content: p}})
		if len(stored) != 0 {
			t.Errorf("FromModelMessages did not filter managed prompt: %q", p)
		}
		restored := session.ToModelMessages([]session.Message{{Role: sdk.RoleSystem, Content: p}})
		if len(restored) != 0 {
			t.Errorf("ToModelMessages did not filter managed prompt: %q", p)
		}
	}

	customSystem := "custom instructions for project"
	storedCustom := session.FromModelMessages([]sdk.Message{{Role: sdk.RoleSystem, Content: customSystem}})
	if len(storedCustom) != 1 || storedCustom[0].Content != customSystem {
		t.Errorf("FromModelMessages filtered custom system prompt: %+v", storedCustom)
	}
	restoredCustom := session.ToModelMessages(storedCustom)
	if len(restoredCustom) != 1 || restoredCustom[0].Content != customSystem {
		t.Errorf("ToModelMessages filtered custom system prompt: %+v", restoredCustom)
	}
}

func TestToolCallCompactionInHistory(t *testing.T) {
	rawResult, _ := json.Marshal(tool.Result{
		ToolName: "read_file",
		CallID:   "call-1",
		Output:   "package main",
	})
	messages := []session.Message{
		{
			Role: sdk.RoleAssistant,
			ToolCalls: []session.ToolCall{
				{ID: "call-1", Name: "read_file"},
			},
		},
		{
			Role:       sdk.RoleTool,
			ToolCallID: "call-1",
			ToolName:   "read_file",
			Content:    string(rawResult),
		},
	}
	restored := session.ToModelMessages(messages)
	if len(restored) != 1 {
		t.Fatalf("restored length = %d, want 1", len(restored))
	}
	if restored[0].Role != sdk.RoleAssistant {
		t.Errorf("restored role = %s, want assistant", restored[0].Role)
	}
	if !strings.Contains(restored[0].Content, "package main") {
		t.Errorf("restored content = %q, want containing 'package main'", restored[0].Content)
	}
}

func TestValidateID(t *testing.T) {
	validIDs := []string{"session-1", "abc_123", "Session.Test", "12345", "a-b_c.d"}
	for _, id := range validIDs {
		if err := session.ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) = %v, want nil", id, err)
		}
	}

	invalidIDs := []string{
		"", "   ", ".", "..", "../etc", "dir/sub", "session 1", "session*bad", "session#1",
	}
	for _, id := range invalidIDs {
		if err := session.ValidateID(id); err == nil {
			t.Errorf("ValidateID(%q) = nil, want error", id)
		}
	}
}

func TestNormalizeLoadedState(t *testing.T) {
	_, err := session.NormalizeLoadedState("s-1", session.State{Version: 99})
	if err == nil {
		t.Fatal("NormalizeLoadedState(Version: 99) = nil, want error")
	}

	_, err = session.NormalizeLoadedState("s-1", session.State{Version: 1, PermissionMode: "invalid-mode"})
	if err == nil {
		t.Fatal("NormalizeLoadedState(invalid mode) = nil, want error")
	}

	_, err = session.NormalizeLoadedState("s-1", session.State{Version: 1, PermissionMode: "ask", ReasoningEffort: "unknown"})
	if err == nil {
		t.Fatal("NormalizeLoadedState(invalid reasoning) = nil, want error")
	}

	now := time.Now().UTC()
	validState := session.State{
		Version:         1,
		PermissionMode:  "ask",
		ReasoningEffort: "high",
		UpdatedAt:       now,
		Messages: []session.Message{
			{Role: sdk.RoleUser, Content: "test message"},
		},
	}
	normalized, err := session.NormalizeLoadedState("workspace-target-123", validState)
	if err != nil {
		t.Fatalf("NormalizeLoadedState error = %v", err)
	}
	if normalized.SessionID != "workspace-target-123" {
		t.Errorf("SessionID = %q, want workspace-target-123", normalized.SessionID)
	}
	if normalized.WorkspaceKey != "target" {
		t.Errorf("WorkspaceKey = %q, want target", normalized.WorkspaceKey)
	}
	if normalized.CreatedAt != now {
		t.Errorf("CreatedAt = %v, want %v", normalized.CreatedAt, now)
	}
}

func TestPrepareStateForSave(t *testing.T) {
	fixedTime := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	input := session.State{
		PermissionMode:  "always-approve",
		ReasoningEffort: "medium",
		Messages: []session.Message{
			{Role: sdk.RoleUser, Content: "save me"},
		},
	}
	existing := &session.State{
		CreatedAt:     time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		WorkspaceName: "my-project",
	}

	prepared, err := session.PrepareStateForSave("session-save-1", input, existing, fixedTime)
	if err != nil {
		t.Fatalf("PrepareStateForSave error = %v", err)
	}
	if prepared.Version != 1 {
		t.Errorf("Version = %d, want 1", prepared.Version)
	}
	if prepared.SessionID != "session-save-1" {
		t.Errorf("SessionID = %q, want session-save-1", prepared.SessionID)
	}
	if prepared.CreatedAt != existing.CreatedAt {
		t.Errorf("CreatedAt = %v, want preserved %v", prepared.CreatedAt, existing.CreatedAt)
	}
	if prepared.WorkspaceName != "my-project" {
		t.Errorf("WorkspaceName = %q, want my-project", prepared.WorkspaceName)
	}
	if prepared.UpdatedAt != fixedTime {
		t.Errorf("UpdatedAt = %v, want %v", prepared.UpdatedAt, fixedTime)
	}
}

func TestPreview(t *testing.T) {
	if got := session.Preview(nil); got != "" {
		t.Errorf("Preview(nil) = %q, want empty", got)
	}

	messages := []session.Message{
		{Role: sdk.RoleSystem, Content: "system instruction"},
		{Role: sdk.RoleAssistant, Content: "assistant reply"},
	}
	if got := session.Preview(messages); got != "" {
		t.Errorf("Preview(non-user) = %q, want empty", got)
	}

	messages = append(messages, session.Message{Role: sdk.RoleUser, Content: "  build   the app   "})
	if got := session.Preview(messages); got != "build the app" {
		t.Errorf("Preview = %q, want 'build the app'", got)
	}

	longText := strings.Repeat("a", 150)
	messages = []session.Message{{Role: sdk.RoleUser, Content: longText}}
	got := session.Preview(messages)
	if len([]rune(got)) != 100 {
		t.Errorf("len(runes) = %d, want 100", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("Preview does not end with ellipsis: %q", got)
	}
}
