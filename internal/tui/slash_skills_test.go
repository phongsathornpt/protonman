package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
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
		s := skill.Skill{
			Name:         "pdf-processing",
			Description:  "Extract PDF text",
			Scope:        skill.ScopeUser,
			Location:     "/home/user/.agents/skills/pdf-processing/SKILL.md",
			BaseDir:      "/home/user/.agents/skills/pdf-processing",
			Instructions: "# PDF Processing Guide\nExtracting text.",
			Resources:    []string{"scripts/extract.py"},
		}
		model.skills = skill.NewRegistry(s)

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
		if !strings.Contains(info, "skill: pdf-processing") {
			t.Fatalf("expected 'skill: pdf-processing' in infoView(), got: %s", info)
		}

		// Multiple active skills show count
		_ = model.skills.Register(skill.Skill{Name: "git-helper", Description: "git"})
		model.skills.MarkActivated("git-helper")
		info = model.infoView()
		if !strings.Contains(info, "2 skills active") {
			t.Fatalf("expected '2 skills active' in infoView(), got: %s", info)
		}

		// Deactivate
		model.skills.Deactivate("pdf-processing")
		model.skills.Deactivate("git-helper")
		info = model.infoView()
		if strings.Contains(info, "active") {
			t.Fatalf("expected no active skill chip when 0 skills active, got: %s", info)
		}
	})

	t.Run("unified skill commands and deactivation verbs", func(t *testing.T) {
		model.skills.MarkActivated("pdf-processing")

		// /skill active works identically to /skills active
		model.executeCommand("/skill active")
		if !strings.Contains(model.viewport.View(), "Active Agent Skills (1):") {
			t.Fatalf("expected /skill active to list active skills")
		}

		// Duplicate activation guard
		initialMsgCount := len(model.messages)
		model.executeCommand("/skill pdf-processing")
		if !strings.Contains(model.viewport.View(), "is already active") {
			t.Fatalf("expected already active message on duplicate activation")
		}
		if len(model.messages) != initialMsgCount {
			t.Fatalf("messages count increased on duplicate activation: %d != %d", len(model.messages), initialMsgCount)
		}

		// /skill deactivate
		model.executeCommand("/skill deactivate pdf-processing")
		if model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be deactivated")
		}
		if !strings.Contains(model.viewport.View(), "Skill \"pdf-processing\" deactivated.") {
			t.Fatalf("expected deactivated message")
		}

		// /skills toggle works
		model.executeCommand("/skills toggle pdf-processing")
		if !model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be activated via /skills toggle")
		}

		// /skill disable
		model.executeCommand("/skill disable pdf-processing")
		if model.skills.IsActivated("pdf-processing") {
			t.Fatalf("expected skill to be deactivated via /skill disable")
		}
	})
}

