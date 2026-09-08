package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/phongsathornpt/proton/internal/feature/skill"
)

func TestSelectSubagentSkillsFiltersByTaskAndProfile(t *testing.T) {
	catalog := skill.NewRegistry(
		testSkill("go-review", "Review Go concurrency and correctness", skill.ScopeUser, nil),
		testSkill("postgres", "Inspect PostgreSQL transactions", skill.ScopeProject, nil),
		testSkill("pdf-tools", "Work with PDF documents", skill.ScopeProject, nil),
		testSkill("dex-only", "Audit security regressions", skill.ScopeUser, map[string]any{
			"proton": map[string]any{"profiles": []any{"dex"}},
		}),
	)
	selected := selectSubagentSkills(catalog, Request{
		Profile: ProfileIntelligence,
		Task:    "review Go transaction code for concurrency and security regressions",
		Context: "internal/store/tx.go",
	})
	names := selectedSkillNames(selected)
	for _, want := range []string{"go-review", "dex-only"} {
		if !containsString(names, want) {
			t.Fatalf("selected skills = %v, missing %q", names, want)
		}
	}
	if containsString(names, "pdf-tools") {
		t.Fatalf("unrelated skill leaked into catalog: %v", names)
	}

	pow := selectSubagentSkills(catalog, Request{Profile: ProfileStrength, Task: "security regression"})
	if containsString(selectedSkillNames(pow), "dex-only") {
		t.Fatalf("profile-incompatible skill leaked into POW catalog: %v", selectedSkillNames(pow))
	}
}

func TestSelectSubagentSkillsEnforcesCatalogLimits(t *testing.T) {
	skills := make([]skill.Skill, 0, 20)
	for i := 0; i < 20; i++ {
		skills = append(skills, testSkill(
			fmt.Sprintf("go-skill-%02d", i),
			"Go implementation "+strings.Repeat("x", 900),
			skill.ScopeUser,
			nil,
		))
	}
	selected := selectSubagentSkills(skill.NewRegistry(skills...), Request{Profile: ProfileStrength, Task: "implement Go code"})
	items := selected.Catalog()
	if len(items) > subagentSkillCatalogMaxItems {
		t.Fatalf("selected %d skills, max %d", len(items), subagentSkillCatalogMaxItems)
	}
	used := 0
	for _, item := range selected.List() {
		used += skillCatalogItemBytes(item)
	}
	if used > subagentSkillCatalogMaxBytes {
		t.Fatalf("catalog bytes = %d, max %d", used, subagentSkillCatalogMaxBytes)
	}
}

func TestSkillBudgetForProfile(t *testing.T) {
	cases := map[Profile]subagentSkillBudget{
		ProfileStrength: {maxActive: 2, maxInstructionBytes: 8 * 1024},
		ProfileAgility: {maxActive: 3, maxInstructionBytes: 12 * 1024},
		ProfileIntelligence: {maxActive: 3, maxInstructionBytes: 16 * 1024},
	}
	for profile, want := range cases {
		if got := skillBudgetForProfile(profile); got != want {
			t.Fatalf("skillBudgetForProfile(%q) = %+v, want %+v", profile, got, want)
		}
	}
}

func TestSelectSubagentSkillsReturnsEmptyForIrrelevantCatalog(t *testing.T) {
	catalog := skill.NewRegistry(testSkill("pdf-tools", "Work with PDF documents", skill.ScopeProject, nil))
	selected := selectSubagentSkills(catalog, Request{Profile: ProfileAgility, Task: "inspect Go scheduler code"})
	if got := len(selected.List()); got != 0 {
		t.Fatalf("selected %d irrelevant skills, want 0", got)
	}
}

func testSkill(name, description string, scope skill.Scope, metadata map[string]any) skill.Skill {
	return skill.Skill{
		Name: name, Description: description, Scope: scope, Metadata: metadata,
		Location: "/skills/" + name + "/SKILL.md", BaseDir: "/skills/" + name,
		Instructions: "Follow " + name + " instructions.",
	}
}

func selectedSkillNames(registry *skill.Registry) []string {
	if registry == nil {
		return nil
	}
	items := registry.List()
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Name)
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
