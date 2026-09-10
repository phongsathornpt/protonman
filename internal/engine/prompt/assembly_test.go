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
		ModelPromptHints:    []string{"model-stable guidance"},
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
		"# Model Guidance",
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

func TestWorkspaceChangePreservesPromptPrefixUntilWorkspaceSection(t *testing.T) {
	base := Spec{
		Role:                "bounded role",
		ActiveGoal:          "keep goal stable",
		ModelPromptHints:    []string{"stable model guidance"},
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
