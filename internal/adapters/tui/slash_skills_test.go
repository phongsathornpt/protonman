package tui

import (
	"context"
	"strings"
	"testing"

	applicationskill "github.com/projectTHORN/proton/internal/application/skill"
	"github.com/projectTHORN/proton/internal/domain/permission"
	domainskill "github.com/projectTHORN/proton/internal/domain/skill"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

func TestSlashSkills(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{
		Name:        "read_file",
		Kind:        tool.KindRead,
		Description: "read",
	}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")

	t.Run("no skills registered", func(t *testing.T) {
		model.executeCommand("/skills")
		content := model.viewport.View()
		if !strings.Contains(content, "No agent skills discovered") {
			t.Fatalf("expected 'No agent skills discovered', got: %s", content)
		}
	})

	t.Run("skills registered", func(t *testing.T) {
		s := domainskill.Skill{
			Name:         "pdf-processing",
			Description:  "Extract PDF text",
			Scope:        domainskill.ScopeUser,
			Location:     "/home/user/.agents/skills/pdf-processing/SKILL.md",
			BaseDir:      "/home/user/.agents/skills/pdf-processing",
			Instructions: "# PDF Processing Guide\nExtracting text.",
			Resources:    []string{"scripts/extract.py"},
		}
		model.skills = applicationskill.NewRegistry(s)

		model.executeCommand("/skills")
		content := model.viewport.View()
		if !strings.Contains(content, "Available Agent Skills:") || !strings.Contains(content, "pdf-processing") {
			t.Fatalf("expected skill list in viewport, got: %s", content)
		}
	})

	t.Run("skill activation", func(t *testing.T) {
		// Missing arg
		model.executeCommand("/skill")
		if !strings.Contains(model.viewport.View(), "usage: /skill <name>") {
			t.Fatalf("expected usage error")
		}

		// Unknown skill
		model.executeCommand("/skill nonexistent")
		if !strings.Contains(model.viewport.View(), "skill \"nonexistent\" not found") {
			t.Fatalf("expected not found error")
		}

		// Valid skill activation
		model.executeCommand("/skill pdf-processing")
		content := model.viewport.View()
		if !strings.Contains(content, "Activated skill pdf-processing [user]:") {
			t.Fatalf("expected activation message in viewport, got: %s", content)
		}
		if !strings.Contains(content, "scripts/extract.py") {
			t.Fatalf("expected bundled resource in viewport, got: %s", content)
		}
		if !model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be marked activated")
		}
		if len(model.messages) == 0 || !strings.Contains(model.messages[len(model.messages)-1].Content, "Activated skill pdf-processing") {
			t.Fatalf("expected skill instruction message to be appended to messages")
		}
	})
}
