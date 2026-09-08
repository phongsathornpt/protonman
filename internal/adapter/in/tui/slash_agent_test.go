package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/tool"
)

func TestSlashAgent(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{
		Name:        "read_file",
		Kind:        tool.KindRead,
		Description: "read",
	}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	bModel := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")

	t.Run("default agent display", func(t *testing.T) {
		bModel.executeCommand("/agent")
		content := bModel.viewport.View()
		if !strings.Contains(content, "Active agent profile") {
			t.Fatalf("expected 'Active agent profile', got: %s", content)
		}
		if !strings.Contains(content, "universal") || !strings.Contains(content, "strength") || !strings.Contains(content, "agility") || !strings.Contains(content, "intelligence") {
			t.Fatalf("expected Dota attribute profiles in list, got: %s", content)
		}
	})

	t.Run("switch to dex profile", func(t *testing.T) {
		bModel.executeCommand("/agent dex")
		if got, want := bModel.agentProfile, "intelligence"; got != want {
			t.Fatalf("bModel.agentProfile = %q, want %q", got, want)
		}
		content := bModel.viewport.View()
		if !strings.Contains(content, "Agent profile switched to intelligence") {
			t.Fatalf("expected switch confirmation, got: %s", content)
		}
		if len(bModel.messages) != 0 {
			t.Fatalf("profile switch mutated transcript: %+v", bModel.messages)
		}
	})

	t.Run("switch to pow profile", func(t *testing.T) {
		bModel.executeCommand("/agent pow")
		if got, want := bModel.agentProfile, "strength"; got != want {
			t.Fatalf("bModel.agentProfile = %q, want %q", got, want)
		}
		if len(bModel.messages) != 0 {
			t.Fatalf("profile switch mutated transcript: %+v", bModel.messages)
		}
	})

	t.Run("reject invalid profile", func(t *testing.T) {
		bModel.executeCommand("/agent invalid_profile")
		content := bModel.viewport.View()
		if !strings.Contains(content, "unknown agent profile") {
			t.Fatalf("expected unknown agent profile error, got: %s", content)
		}
	})
}
