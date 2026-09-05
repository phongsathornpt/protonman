package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
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
		if !strings.Contains(content, "pow") || !strings.Contains(content, "dex") || !strings.Contains(content, "int") {
			t.Fatalf("expected pow, dex, int profiles in list, got: %s", content)
		}
	})

	t.Run("switch to dex profile", func(t *testing.T) {
		bModel.executeCommand("/agent dex")
		if got, want := bModel.agentProfile, "dex"; got != want {
			t.Fatalf("bModel.agentProfile = %q, want %q", got, want)
		}
		content := bModel.viewport.View()
		if !strings.Contains(content, "Agent profile switched to dex") {
			t.Fatalf("expected switch confirmation, got: %s", content)
		}
		if len(bModel.messages) == 0 || bModel.messages[0].Role != model.RoleSystem {
			t.Fatalf("expected system message to be set in transcript, got: %+v", bModel.messages)
		}
		if !strings.Contains(bModel.messages[0].Content, "DEX Mode") {
			t.Fatalf("expected DEX Mode prompt, got: %s", bModel.messages[0].Content)
		}
	})

	t.Run("switch to pow profile", func(t *testing.T) {
		bModel.executeCommand("/agent pow")
		if got, want := bModel.agentProfile, "pow"; got != want {
			t.Fatalf("bModel.agentProfile = %q, want %q", got, want)
		}
		if !strings.Contains(bModel.messages[0].Content, "POW Mode") {
			t.Fatalf("expected POW Mode prompt, got: %s", bModel.messages[0].Content)
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
