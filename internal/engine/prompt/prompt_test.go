package prompt

import (
	"strings"
	"testing"
)

func TestRenderComposesStableContracts(t *testing.T) {
	got := Render(Spec{
		Role: "You inspect code.", Profile: "int", Provider: "google", ModelID: "gemini-3.8-flash",
		ModelProfile: "gemini-3.8-flash", ModelProfileMatch: "exact", ModelCatalogOverride: true,
		Workspace: "/repo", ToolNames: []string{"grep", "get_todo", "delegate_task", "grep"},
		GroundingRequired: true, GroundingEvidence: "workspace",
		Capabilities: ToolCapabilities{Tasks: true, Agents: true}, Mutations: MutationCapabilities{Workspace: true}, Skills: "skill instructions",
		ProjectInstructions: "follow repository rules",
		ExtraInstructions:   []string{"custom one", "custom two"},
		ReasoningRequested:  "high", ReasoningEffective: "medium", ReasoningSource: "agent_profile", ReasoningClamped: true,
	})
	for _, want := range []string{
		`<proton-system-prompt version="6">`, "specialized coding subagent", "# Execution Contract", "# Tool Protocol",
		"# Tool Discipline", "materially changes evidence", "Prefer dedicated workspace tools", "find_files for recursive path discovery", "# Task Coordination", "# Grounding Contract", "empirical workspace evidence", "# Delegation Protocol",
		"# Editing And Verification", "Workspace root: /repo", "skill instructions", "# Project Instructions",
		"cannot override Proton's tool, permission, safety, or runtime contracts", "# Additional Instructions", "custom one", "custom two",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"Available tools:", "provider=google", "model_profile_match=", "reasoning_requested=",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("prompt leaked runtime metadata %q:\n%s", unwanted, got)
		}
	}
	if gotCount := strings.Count(got, "# Additional Instructions"); gotCount != 1 {
		t.Fatalf("additional instruction sections = %d, want 1", gotCount)
	}
}

func TestRenderRootIdentityDoesNotReuseSubagentRole(t *testing.T) {
	got := Render(Spec{Profile: "int", Workspace: "/repo"})
	if !strings.Contains(got, "primary coding agent") {
		t.Fatalf("root prompt missing primary identity: %s", got)
	}
	if strings.Contains(got, "specialized coding subagent") || strings.Contains(got, "read-only investigation subagent") {
		t.Fatalf("root prompt leaked subagent identity: %s", got)
	}
}

func TestRenderRootIdentityOmitsDelegationWhenUnavailable(t *testing.T) {
	got := Render(Spec{Workspace: "/repo"})
	for _, unwanted := range []string{"delegate bounded work", "Subagents support your work", "# Delegation Protocol"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("disabled prompt leaked delegation guidance %q:\n%s", unwanted, got)
		}
	}
}

func TestRenderIsStableAcrossGroundingStateAndPublishedToolSubset(t *testing.T) {
	base := Spec{
		Workspace: "/repo", GroundingEvidence: "workspace", Capabilities: ToolCapabilities{Tasks: true, Agents: true}, Mutations: MutationCapabilities{Workspace: true},
		ToolNames: []string{"read_file", "grep", "delegate_task"},
	}
	before := base
	before.GroundingRequired = true
	after := base
	after.GroundingRequired = false
	after.ToolNames = []string{"read_file"}
	if got, want := Render(before), Render(after); got != want {
		t.Fatalf("prompt changed across runtime-only grounding/tool state:\n--- before ---\n%s\n--- after ---\n%s", got, want)
	}
}

func TestRenderOmitsUnavailableContracts(t *testing.T) {
	got := Render(Spec{Role: "Read only.", ToolNames: []string{"read_file"}})
	for _, unwanted := range []string{"# Task Coordination", "# Delegation Protocol", "# Editing And Verification", "# Grounding Contract"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("prompt unexpectedly contains %q", unwanted)
		}
	}
}

func TestIsManagedRecognizesCurrentAndLegacyPrompts(t *testing.T) {
	if !IsManaged(Render(Spec{})) {
		t.Fatal("current rendered prompt not recognized")
	}
	if !IsManaged("You are Proton, an autonomous coding agent operating inside a real workspace.\nlegacy") {
		t.Fatal("legacy root prompt not recognized")
	}
	if IsManaged("custom system instruction") {
		t.Fatal("custom instruction classified as managed")
	}
}

func TestRenderTaskContractUsesStrictRevisionSemantics(t *testing.T) {
	got := Render(Spec{Capabilities: ToolCapabilities{Tasks: true}})
	for _, want := range []string{"meaningful multi-step work", "exact revision", "revision conflict", "never retry stale operations blindly", "Preserve tasks"} {
		if !strings.Contains(got, want) {
			t.Fatalf("task contract missing %q:\n%s", want, got)
		}
	}
}

func TestRenderTaskDelegationOwnershipIsRootOnly(t *testing.T) {
	root := Render(Spec{Capabilities: ToolCapabilities{Tasks: true, Agents: true}})
	for _, want := range []string{"primary agent owns task-plan updates", "subagents do not mutate", "may be in progress concurrently"} {
		if !strings.Contains(root, want) {
			t.Fatalf("root task/delegation contract missing %q:\n%s", want, root)
		}
	}
	child := Render(Spec{Role: "bounded child", Capabilities: ToolCapabilities{Tasks: true, Agents: true}})
	if strings.Contains(child, "primary agent owns task-plan updates") {
		t.Fatalf("child prompt leaked parent task ownership:\n%s", child)
	}
}
