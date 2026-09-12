package prompt

import (
	"strings"
	"testing"
)

func TestRenderSectionsUsesDeterministicOrder(t *testing.T) {
	got := renderSections([]Section{
		{Name: "zeta", Order: 10, Text: "zeta"},
		{Name: "beta", Order: 0, Text: "beta"},
		{Name: "alpha", Order: 10, Text: "alpha"},
		{Name: "empty", Order: -100, Text: "   "},
	})
	if want := "beta\n\nalpha\n\nzeta"; got != want {
		t.Fatalf("renderSections() = %q, want %q", got, want)
	}
}

func TestRenderPlacesVolatileWorkspaceAfterReusableSections(t *testing.T) {
	got := Render(Spec{
		Role:                "bounded implementation role",
		ActiveGoal:          "finish prompt cache work",
		Workspace:           "/volatile/workspace",
		ProjectInstructions: "project-stable instructions",
		Skills:              "session skill context",
		Capabilities:        ToolCapabilities{Tasks: true, Agents: true},
		Mutations:           MutationCapabilities{Source: true},
		AvailableTools:      []string{"read", "edit", "bash", "todo", "subagent"},
	})

	workspace := strings.Index(got, "# Workspace")
	if workspace < 0 {
		t.Fatalf("workspace section missing:\n%s", got)
	}
	for _, marker := range []string{
		"# Execution Contract",
		"# Tool Use",
		"# Task Coordination",
		"# Delegation Protocol",
		"# Editing And Verification",
		"# Project Instructions",
		"# Skills",
		"# Role",
		"# Active Goal",
	} {
		index := strings.Index(got, marker)
		if index < 0 {
			t.Fatalf("section %q missing:\n%s", marker, got)
		}
		if index >= workspace {
			t.Fatalf("section %q begins at %d after volatile workspace at %d:\n%s", marker, index, workspace, got)
		}
	}
}

func TestModelPromptHintsDoNotAffectCanonicalPrompt(t *testing.T) {
	base := Spec{
		ProjectInstructions: "stable project instructions",
		AvailableTools:      []string{"read", "bash"},
		Workspace:           "/repo",
	}
	left := base
	left.ModelPromptHints = []string{"gemini-specific guidance"}
	right := base
	right.ModelPromptHints = []string{"different-model guidance"}
	if a, b := Render(left), Render(right); a != b {
		t.Fatalf("model prompt hints changed canonical prompt\n--- left ---\n%s\n--- right ---\n%s", a, b)
	}
	if got := Render(left); strings.Contains(got, "# Model Guidance") || strings.Contains(got, "gemini-specific guidance") {
		t.Fatalf("deprecated model prompt hints leaked into canonical prompt:\n%s", got)
	}
}

func TestWorkspaceChangePreservesPromptPrefixUntilWorkspaceSection(t *testing.T) {
	base := Spec{
		Role:                "bounded role",
		ActiveGoal:          "keep goal stable",
		ProjectInstructions: "stable project instructions",
		Skills:              "stable skills",
		AvailableTools:      []string{"read", "bash"},
	}
	left := base
	left.Workspace = "/repo/a"
	right := base
	right.Workspace = "/repo/b"

	a := Render(left)
	b := Render(right)
	prefix := longestCommonPrefix(a, b)
	workspace := strings.Index(a, "# Workspace")
	if workspace < 0 {
		t.Fatalf("workspace section missing:\n%s", a)
	}
	if prefix < workspace {
		t.Fatalf("workspace change diverged at byte %d before workspace section at %d", prefix, workspace)
	}
}

func longestCommonPrefix(a, b string) int {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}

func TestProjectInstructionsChangePreservesEarlierPrefix(t *testing.T) {
	base := Spec{
		AvailableTools: []string{"read", "bash"},
		Workspace:      "/repo",
	}
	left := base
	left.ProjectInstructions = "project rules alpha"
	right := base
	right.ProjectInstructions = "project rules beta"

	a := Render(left)
	b := Render(right)
	prefix := longestCommonPrefix(a, b)
	boundary := strings.Index(a, "# Project Instructions")
	if boundary < 0 {
		t.Fatalf("project instructions section missing:\n%s", a)
	}
	if prefix < boundary {
		t.Fatalf("project instruction change diverged at byte %d before project section at %d", prefix, boundary)
	}
}

func TestActiveGoalChangePreservesEarlierPrefix(t *testing.T) {
	base := Spec{
		ProjectInstructions: "stable project instructions",
		Skills:              "stable skills",
		Role:                "stable role",
		Workspace:           "/repo",
	}
	left := base
	left.ActiveGoal = "finish alpha"
	right := base
	right.ActiveGoal = "finish beta"

	a := Render(left)
	b := Render(right)
	prefix := longestCommonPrefix(a, b)
	boundary := strings.Index(a, "# Active Goal")
	if boundary < 0 {
		t.Fatalf("active goal section missing:\n%s", a)
	}
	if prefix < boundary {
		t.Fatalf("active goal change diverged at byte %d before active goal section at %d", prefix, boundary)
	}
}

func TestSkillStateChangePreservesEarlierPrefix(t *testing.T) {
	base := Spec{
		ProjectInstructions: "stable project instructions",
		Role:                "stable role",
		ActiveGoal:          "stable goal",
		Workspace:           "/repo",
	}
	left := base
	left.Skills = "skill alpha"
	right := base
	right.Skills = "skill beta"

	a := Render(left)
	b := Render(right)
	prefix := longestCommonPrefix(a, b)
	boundary := strings.Index(a, "# Skills")
	if boundary < 0 {
		t.Fatalf("skills section missing:\n%s", a)
	}
	if prefix < boundary {
		t.Fatalf("skill change diverged at byte %d before skills section at %d", prefix, boundary)
	}
}

func TestSubagentRoleChangePreservesSharedPrefix(t *testing.T) {
	base := Spec{
		ProjectInstructions: "stable project instructions",
		Skills:              "stable skills",
		ActiveGoal:          "stable goal",
		Workspace:           "/repo",
	}
	left := base
	left.Role = "implementation subagent role"
	right := base
	right.Role = "investigation subagent role"

	a := Render(left)
	b := Render(right)
	prefix := longestCommonPrefix(a, b)
	boundary := strings.Index(a, "# Role")
	if boundary < 0 {
		t.Fatalf("role section missing:\n%s", a)
	}
	if prefix < boundary {
		t.Fatalf("subagent role change diverged at byte %d before role section at %d", prefix, boundary)
	}
}

func TestEquivalentToolSetsRenderIdenticallyRegardlessOfInputOrder(t *testing.T) {
	left := Spec{AvailableTools: []string{"read", "grep", "find", "ls", "git", "math", "edit", "web", "bash"}}
	right := Spec{AvailableTools: []string{"bash", "web", "edit", "math", "git", "ls", "find", "grep", "read"}}
	if a, b := Render(left), Render(right); a != b {
		t.Fatalf("equivalent tool sets produced different prompt output\n--- left ---\n%s\n--- right ---\n%s", a, b)
	}
}

func TestPromptRenderIsByteStableAcrossEquivalentSpecs(t *testing.T) {
	spec := Spec{
		Role:                "stable role",
		ActiveGoal:          "stable goal",
		Workspace:           "/repo",
		AvailableTools:      []string{"read", "bash", "web"},
		GroundingEvidence:   "workspace",
		Capabilities:        ToolCapabilities{Tasks: true, Agents: true, MCP: true},
		Mutations:           MutationCapabilities{Source: true},
		Skills:              "stable skills",
		ProjectInstructions: "stable project instructions",
		ExtraInstructions:   []string{"extra one", "extra two"},
	}
	first := Render(spec)
	for i := 0; i < 100; i++ {
		if got := Render(spec); got != first {
			t.Fatalf("render %d was not byte-stable", i)
		}
	}
}
