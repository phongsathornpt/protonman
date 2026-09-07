package agentprompt

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
		TaskPlanEnabled: true, DelegationEnabled: true, MutationEnabled: true, Skills: "skill instructions",
		ProjectInstructions: "follow repository rules",
		ExtraInstructions:   []string{"custom one", "custom two"},
		ReasoningRequested:  "high", ReasoningEffective: "medium", ReasoningSource: "agent_profile", ReasoningClamped: true,
	})
	for _, want := range []string{
		`<proton-system-prompt version="5">`, "specialized coding subagent", "# Execution Contract", "# Tool Protocol",
		"# Tool Discipline", "materially changes evidence", "# Task Coordination", "# Grounding Contract", "empirical workspace evidence", "# Delegation Protocol",
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

func TestRenderIsStableAcrossGroundingStateAndPublishedToolSubset(t *testing.T) {
	base := Spec{
		Workspace: "/repo", GroundingEvidence: "workspace", TaskPlanEnabled: true, DelegationEnabled: true, MutationEnabled: true,
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
