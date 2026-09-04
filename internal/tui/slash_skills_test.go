package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

	t.Run("skill name autocomplete in composer", func(t *testing.T) {
		model.bottom.remove(skillsViewID)
		model.bottom.prompt().SetValue("/skill ")
		if !model.slashOpen() {
			t.Fatal("slash dropdown did not open for /skill ")
		}
		matches := model.slashMatches()
		if len(matches) == 0 {
			t.Fatal("expected matches for /skill ")
		}

		// Filter by prefix
		model.bottom.prompt().SetValue("/skill pd")
		matches = model.slashMatches()
		if len(matches) != 1 || matches[0].name != "pdf-processing" {
			t.Fatalf("expected pdf-processing match, got: %#v", matches)
		}

		// Tab accept completes the name
		applied, cmd := model.acceptSlash(false)
		if !applied || cmd != nil {
			t.Fatalf("acceptSlash failed: applied=%v, cmd=%v", applied, cmd)
		}
		if got := model.bottom.prompt().Value(); got != "/skill pdf-processing" {
			t.Fatalf("prompt value after accept = %q, want /skill pdf-processing", got)
		}
	})

	t.Run("interactive bottom-pane skills picker", func(t *testing.T) {
		model.bottom.remove(skillsViewID)
		model.executeCommand("/skills")
		if !model.bottom.has(skillsViewID) {
			t.Fatal("expected skills picker in bottom pane after /skills")
		}

		// View rendered
		rendered := model.bottom.renderTop(model)
		if !strings.Contains(rendered, "Agent Skills") || !strings.Contains(rendered, "pdf-processing") {
			t.Fatalf("unexpected picker render: %s", rendered)
		}

		// Space toggles skill
		wasActive := model.skills.IsActivated("pdf-processing")
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
		model = updated.(*bubbleModel)
		if model.skills.IsActivated("pdf-processing") == wasActive {
			t.Fatalf("spacebar did not toggle skill active status")
		}

		// Esc closes picker
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
		model = updated.(*bubbleModel)
		if model.bottom.has(skillsViewID) {
			t.Fatal("esc did not close skills picker")
		}
	})

	t.Run("ctrl+s shortcut toggles skills picker", func(t *testing.T) {
		model.bottom.remove(skillsViewID)
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		model = updated.(*bubbleModel)
		if !model.bottom.has(skillsViewID) {
			t.Fatal("ctrl+s did not open skills picker")
		}

		// Pressing ctrl+s again closes it
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		model = updated.(*bubbleModel)
		if model.bottom.has(skillsViewID) {
			t.Fatal("second ctrl+s did not close skills picker")
		}
	})

	t.Run("new command resets active skills", func(t *testing.T) {
		model.skills.MarkActivated("pdf-processing")
		if !model.skills.IsActivated("pdf-processing") {
			t.Fatal("expected skill to be active")
		}
		model.executeCommand("/new")
		if model.skills.IsActivated("pdf-processing") {
			t.Fatal("expected /new to clear active skills")
		}
	})

	t.Run("skill activation does not append user message and does not flood instructions", func(t *testing.T) {
		model.messages = nil
		model.skills.Deactivate("pdf-processing")
		model.executeCommand("/skill pdf-processing")
		content := model.viewport.View()
		if !strings.Contains(content, "[x] Activated skill pdf-processing [user]: Extract PDF text") {
			t.Fatalf("expected activation message with description, got: %s", content)
		}
		if strings.Contains(content, "# PDF Processing Guide") {
			t.Fatalf("did not expect raw instructions markdown in viewport")
		}
		if len(model.messages) != 0 {
			t.Fatalf("expected 0 messages appended to model.messages, got %d", len(model.messages))
		}
	})
}

