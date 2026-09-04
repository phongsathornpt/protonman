package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
)

func TestSlashRounds(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{
		Name:        "read_file",
		Kind:        tool.KindRead,
		Description: "read",
	}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")

	t.Run("default rounds display", func(t *testing.T) {
		model.executeCommand("/rounds")
		content := model.viewport.View()
		if !strings.Contains(content, "20 rounds per turn") {
			t.Fatalf("expected '20 rounds per turn', got: %s", content)
		}
	})

	t.Run("set valid rounds", func(t *testing.T) {
		model.executeCommand("/rounds 15")
		if got, want := model.maxRounds, 15; got != want {
			t.Fatalf("model.maxRounds = %d, want %d", got, want)
		}
		content := model.viewport.View()
		if !strings.Contains(content, "Turn round limit set to 15 rounds per turn.") {
			t.Fatalf("expected success message for 15 rounds, got: %s", content)
		}
	})

	t.Run("set unbounded rounds with 0", func(t *testing.T) {
		model.executeCommand("/rounds 0")
		if got, want := model.maxRounds, 0; got != want {
			t.Fatalf("model.maxRounds = %d, want %d", got, want)
		}
		content := model.viewport.View()
		if !strings.Contains(content, "Turn round limit set to unbounded (0).") {
			t.Fatalf("expected unbounded message, got: %s", content)
		}
		// Query current limit when unbounded
		model.executeCommand("/rounds")
		if !strings.Contains(model.viewport.View(), "unbounded (0)") {
			t.Fatalf("expected query to return unbounded (0), got: %s", model.viewport.View())
		}
	})

	t.Run("reject invalid rounds", func(t *testing.T) {
		model.executeCommand("/rounds -5")
		content := model.viewport.View()
		if !strings.Contains(content, "invalid round limit") {
			t.Fatalf("expected error for negative rounds, got: %s", content)
		}

		model.executeCommand("/rounds abc")
		content = model.viewport.View()
		if !strings.Contains(content, "invalid round limit") {
			t.Fatalf("expected error for non-integer rounds, got: %s", content)
		}
	})

	t.Run("max-rounds alias", func(t *testing.T) {
		model.executeCommand("/max-rounds 25")
		if got, want := model.maxRounds, 25; got != want {
			t.Fatalf("model.maxRounds = %d, want %d", got, want)
		}
		content := model.viewport.View()
		if !strings.Contains(content, "Turn round limit set to 25 rounds per turn.") {
			t.Fatalf("expected success message for 25 rounds, got: %s", content)
		}
	})
}
