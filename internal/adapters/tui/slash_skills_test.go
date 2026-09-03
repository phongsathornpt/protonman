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

	t.Run("skills registered with checkbox", func(t *testing.T) {
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
		if !strings.Contains(content, "Agent Skills (0/1 active):") || !strings.Contains(content, "[ ] pdf-processing") {
			t.Fatalf("expected unchecked skill in viewport, got: %s", content)
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
		if !strings.Contains(content, "[x] Activated skill pdf-processing [user]:") {
			t.Fatalf("expected activation message in viewport, got: %s", content)
		}
		if !strings.Contains(content, "scripts/extract.py") {
			t.Fatalf("expected bundled resource in viewport, got: %s", content)
		}
		if !model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be marked activated")
		}

		// Verify /skills now shows [x]
		model.executeCommand("/skills")
		content = model.viewport.View()
		if !strings.Contains(content, "Agent Skills (1/1 active):") || !strings.Contains(content, "[x] pdf-processing") {
			t.Fatalf("expected checked skill in /skills, got: %s", content)
		}

		// Verify /skills active
		model.executeCommand("/skills active")
		content = model.viewport.View()
		if !strings.Contains(content, "Active Agent Skills (1):") || !strings.Contains(content, "[x] pdf-processing") {
			t.Fatalf("expected active skills list, got: %s", content)
		}
	})

	t.Run("skill toggle", func(t *testing.T) {
		// Toggle to inactive
		model.executeCommand("/skill toggle pdf-processing")
		content := model.viewport.View()
		if !strings.Contains(content, "[ ] Skill \"pdf-processing\" deactivated.") {
			t.Fatalf("expected deactivated message, got: %s", content)
		}
		if model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be deactivated")
		}

		// Verify /skills active shows no active skills
		model.executeCommand("/skills active")
		content = model.viewport.View()
		if !strings.Contains(content, "No active agent skills in this session.") {
			t.Fatalf("expected no active skills message, got: %s", content)
		}

		// Toggle back to active
		model.executeCommand("/skill toggle pdf-processing")
		content = model.viewport.View()
		if !strings.Contains(content, "[x] Skill \"pdf-processing\" activated.") {
			t.Fatalf("expected activated message, got: %s", content)
		}
		if !model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be activated again")
		}
	})

	t.Run("status bar info view shows active skills chip", func(t *testing.T) {
		// pdf-processing is currently active
		info := model.infoView()
		if !strings.Contains(info, "1 skill active") {
			t.Fatalf("expected '1 skill active' in infoView(), got: %s", info)
		}

		// Deactivate
		model.skills.Deactivate("pdf-processing")
		info = model.infoView()
		if strings.Contains(info, "skill active") {
			t.Fatalf("expected no active skill chip when 0 skills active, got: %s", info)
		}
	})
}
