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

func TestRenderPlacesVolatileContextAfterReusablePrefix(t *testing.T) {
	got := Render(Spec{
		Role:                "bounded implementation role",
		ActiveGoal:          "finish prompt cache work",
		Workspace:           "/volatile/workspace",
		GroundingEvidence:   "workspace",
		ProjectInstructions: "project-stable instructions",
		ExtraInstructions:   []string{"session-stable instruction"},
		Skills:              "session skill context",
		Capabilities:        ToolCapabilities{Tasks: true, Agents: true},
		Mutations:           MutationCapabilities{Source: true},
		AvailableTools:      []string{"read", "edit", "bash", "todo", "subagent"},
	})

	role := strings.Index(got, "# Role")
	if role < 0 {
		t.Fatalf("role section missing:\n%s", got)
	}
	for _, marker := range []string{
		"# Execution Contract",
		"# Tool Use",
		"# Workspace",
		"# Task Coordination",
		"# Delegation Protocol",
		"# Editing And Verification",
		"# Project Instructions",
		"# Additional Instructions",
	} {
		index := strings.Index(got, marker)
		if index < 0 {
			t.Fatalf("section %q missing:\n%s", marker, got)
		}
		if index >= role {
			t.Fatalf("reusable section %q begins at %d after volatile suffix starts at %d:\n%s", marker, index, role, got)
		}
	}

	last := role
	for _, marker := range []string{"# Active Goal", "# Skills", "# Grounding Contract"} {
		index := strings.Index(got, marker)
		if index <= last {
			t.Fatalf("volatile section %q is not ordered after previous boundary in prompt:\n%s", marker, got)
		}
		last = index
	}
}

func TestCanonicalPromptContainsNoModelGuidanceSection(t *testing.T) {
	got := Render(Spec{
		ProjectInstructions: "stable project instructions",
		AvailableTools:      []string{"read", "bash"},
		Workspace:           "/repo",
	})
	if strings.Contains(got, "# Model Guidance") {
		t.Fatalf("canonical prompt contains model-specific guidance section:\n%s", got)
	}
}

func TestWorkspaceValueDoesNotAffectCanonicalPrompt(t *testing.T) {
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

	if a, b := Render(left), Render(right); a != b {
		t.Fatalf("absolute workspace value changed canonical prompt\n--- left ---\n%s\n--- right ---\n%s", a, b)
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

func TestGroundingChangePreservesEarlierPromptPrefix(t *testing.T) {
	base := Spec{
		ProjectInstructions: "stable project instructions",
		Role:                "stable role",
		ActiveGoal:          "stable goal",
		Skills:              "stable skills",
		Workspace:           "/repo",
	}
	left := base
	left.GroundingEvidence = "workspace"
	right := base
	right.GroundingEvidence = "external"

	a := Render(left)
	b := Render(right)
	prefix := longestCommonPrefix(a, b)
	boundary := strings.Index(a, "# Grounding Contract")
	if boundary < 0 {
		t.Fatalf("grounding section missing:\n%s", a)
	}
	if prefix < boundary {
		t.Fatalf("grounding change diverged at byte %d before grounding section at %d", prefix, boundary)
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
