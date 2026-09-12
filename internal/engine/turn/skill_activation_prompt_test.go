package turn

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	skilltool "github.com/phongsathornpt/protonman/internal/adapter/out/tool/skill"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
)

func TestSkillActivationInjectsInstructionsExactlyOnce(t *testing.T) {
	const instructions = "UNIQUE SKILL INSTRUCTION BODY"
	s := skill.Skill{
		Name:         "cache-safe-skill",
		Description:  "Exercise single-source skill prompt injection",
		Scope:        skill.ScopeUser,
		Instructions: instructions,
	}
	registry := skill.NewRegistry(s)
	handler := skilltool.NewActivateSkill(registry)
	args, err := json.Marshal(map[string]any{"name": s.Name})
	if err != nil {
		t.Fatal(err)
	}
	call, err := tool.NewCall("activate-skill", tool.NameSkill, args)
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.Execute(context.Background(), call)
	if err != nil {
		t.Fatalf("activate skill: %v", err)
	}
	if strings.Contains(result.Output, instructions) {
		t.Fatalf("tool result duplicated active skill instructions: %q", result.Output)
	}

	loop := &Loop{skillRegistry: registry}
	section := loop.currentSkillPromptSection()
	if got := strings.Count(section, instructions); got != 1 {
		t.Fatalf("active skill instruction count = %d, want 1; prompt section: %s", got, section)
	}
}
