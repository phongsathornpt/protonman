package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/adapters/tools"
	applicationskill "github.com/projectTHORN/proton/internal/application/skill"
	domainskill "github.com/projectTHORN/proton/internal/domain/skill"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

func TestActivateSkill_Execute(t *testing.T) {
	ctx := context.Background()

	s := domainskill.Skill{
		Name:         "pdf-processing",
		Description:  "Extract text from PDFs",
		Location:     "/home/user/.agents/skills/pdf-processing/SKILL.md",
		BaseDir:      "/home/user/.agents/skills/pdf-processing",
		Scope:        domainskill.ScopeUser,
		Instructions: "# PDF Processing\n\nRun scripts/extract.py to extract text.",
		Resources:    []string{"scripts/extract.py", "references/guide.md"},
	}
	skillReg := applicationskill.NewRegistry(s)
	handler := tools.NewActivateSkill(skillReg)

	t.Run("successful activation", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"name": "pdf-processing"})
		call, err := tool.NewCall("call-1", "activate_skill", args)
		if err != nil {
			t.Fatal(err)
		}

		result, err := handler.Execute(ctx, call)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Failure != nil {
			t.Fatalf("expected success, got failure: %+v", result.Failure)
		}

		// Check output contents
		if !strings.Contains(result.Output, `<skill_content name="pdf-processing">`) {
			t.Errorf("missing skill_content tag: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Run scripts/extract.py to extract text.") {
			t.Errorf("missing instructions in output: %s", result.Output)
		}
		if !strings.Contains(result.Output, "Skill directory: /home/user/.agents/skills/pdf-processing") {
			t.Errorf("missing skill directory in output: %s", result.Output)
		}
		if !strings.Contains(result.Output, "<file>scripts/extract.py</file>") {
			t.Errorf("missing bundled resource in output: %s", result.Output)
		}
		if !strings.Contains(result.Output, "</skill_content>") {
			t.Errorf("missing closing skill_content tag: %s", result.Output)
		}

		// Check that it was marked activated
		if !skillReg.IsActivated("pdf-processing") {
			t.Errorf("expected skill to be marked activated in registry")
		}
	})

	t.Run("skill not found", func(t *testing.T) {
		args, _ := json.Marshal(map[string]any{"name": "non-existent"})
		call, _ := tool.NewCall("call-2", "activate_skill", args)

		_, err := handler.Execute(ctx, call)
		if err == nil {
			t.Fatal("expected error for nonexistent skill")
		}
		if !strings.Contains(err.Error(), "skill \"non-existent\" not found") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("invalid arguments", func(t *testing.T) {
		args := []byte(`{"invalid": true}`)
		call, _ := tool.NewCall("call-3", "activate_skill", args)

		_, err := handler.Execute(ctx, call)
		if err == nil {
			t.Fatal("expected error for missing name argument")
		}
	})
}
