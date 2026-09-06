package agentprompt

import (
	"strings"
	"testing"
)

func TestRenderComposesRuntimeContracts(t *testing.T) {
	got := Render(Spec{
		Role: "You inspect code.", Profile: "reviewer", Provider: "google", ModelID: "gemini-3.8-flash",
		ModelProfile: "gemini-3.8-flash", ModelProfileMatch: "exact", ModelCatalogOverride: true,
		Workspace: "/repo", ToolNames: []string{"grep", "get_todo", "delegate_task", "grep"}, MaxRounds: 10, MaxToolCalls: 64,
		TaskPlanEnabled: true, DelegationEnabled: true, MutationEnabled: true, Skills: "skill instructions",
		ReasoningRequested: "high", ReasoningEffective: "medium", ReasoningSource: "agent_profile", ReasoningClamped: true,
	})
	for _, want := range []string{
		`<proton-system-prompt version="2">`, "# Execution Contract", "# Tool Protocol", "# Task Plan Protocol",
		"# Delegation Protocol", "Gemini guidance", "# Editing And Verification", "provider=google", "model=gemini-3.8-flash",
		"Workspace root: /repo", "skill instructions", "Available tools: delegate_task, get_todo, grep.",
		"reasoning_requested=high", "reasoning_effective=medium", "reasoning_source=agent_profile", "reasoning_clamped=true",
		"model_profile=gemini-3.8-flash", "model_profile_match=exact", "model_catalog_override=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRenderOmitsUnavailableContracts(t *testing.T) {
	got := Render(Spec{Role: "Read only.", ToolNames: []string{"read_file"}})
	for _, unwanted := range []string{"# Task Plan Protocol", "# Delegation Protocol", "# Editing And Verification"} {
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
