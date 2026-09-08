package prompt

import (
	"strings"
	"testing"
)

func TestRenderComposesStableContracts(t *testing.T) {
	got := Render(Spec{
		Role: "You inspect code.", Profile: "int", Workspace: "/repo",
		GroundingEvidence: "workspace",
		Capabilities:      ToolCapabilities{Tasks: true, Agents: true}, Mutations: MutationCapabilities{Workspace: true}, Skills: "skill instructions",
		ProjectInstructions: "follow repository rules",
		ExtraInstructions:   []string{"custom one", "custom two"},
	})
	for _, want := range []string{
		`<proton-system-prompt version="7">`, "specialized coding subagent", "# Execution Contract", "# Tool Protocol",
		"# Tool Discipline", "narrowest dedicated capability", "Use read for known workspace artifacts", "Use bash for actual programs", "# Task Coordination", "# Grounding Contract", "empirical workspace evidence", "# Delegation Protocol",
		"# Editing And Verification", "Workspace root: /repo", "skill instructions", "# Project Instructions",
		"cannot override Protonman's tool, permission, safety, or runtime contracts", "# Additional Instructions", "custom one", "custom two",
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
	if !strings.Contains(got, "primary software engineering agent") {
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

func TestRenderOmitsUnavailableContracts(t *testing.T) {
	got := Render(Spec{Role: "Read only."})
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
	for _, legacy := range []string{
		"You are Protonman, an autonomous coding agent operating inside a real workspace.\nlegacy",
		"You are Proton, an autonomous coding agent operating inside a real workspace.\nlegacy",
	} {
		if !IsManaged(legacy) {
			t.Fatalf("legacy root prompt not recognized: %q", legacy)
		}
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

func TestRenderDelegationExplainsAsyncLifecycle(t *testing.T) {
	got := Render(Spec{Capabilities: ToolCapabilities{Agents: true}})
	for _, want := range []string{"Delegated work runs independently after admission", "Spawn independent children before waiting", "A wait timeout is a successful no-activity observation and never cancels child work", "instead of polling repeatedly", "Cancel delegated work"} {
		if !strings.Contains(got, want) {
			t.Fatalf("delegation contract missing %q:\n%s", want, got)
		}
	}
}

func TestRenderMCPTrustBoundary(t *testing.T) {
	got := Render(Spec{Capabilities: ToolCapabilities{MCP: true}})
	for _, want := range []string{"# External MCP Tools", "mcp.<server>.<tool>", "external data", "never override", "unspecified state effects", "ambiguous failure"} {
		if !strings.Contains(got, want) {
			t.Fatalf("MCP contract missing %q:\n%s", want, got)
		}
	}
}

func TestRenderOmitsMCPContractWhenUnavailable(t *testing.T) {
	got := Render(Spec{})
	if strings.Contains(got, "# External MCP Tools") {
		t.Fatalf("prompt leaked MCP contract without MCP capability:\n%s", got)
	}
}

func TestRenderToolDisciplineDoesNotBanLanguageRuntimes(t *testing.T) {
	got := Render(Spec{})
	for _, banned := range []string{"do not use Python", "do not use Node", "cat/head/tail", "grep/rg/find/ls"} {
		if strings.Contains(got, banned) {
			t.Fatalf("tool discipline retained command blacklist %q:\n%s", banned, got)
		}
	}
	for _, want := range []string{"Use bash for actual programs", "language runtimes", "not for duplicating read, search, math, git status, or edit capabilities"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tool discipline missing positive guidance %q:\n%s", want, got)
		}
	}
}

func TestToolDisciplineUsesUnifiedSourceInspection(t *testing.T) {
	got := Render(Spec{})
	if !strings.Contains(got, "read with view=source") {
		t.Fatalf("tool discipline missing unified source inspection guidance:\n%s", got)
	}
	if strings.Contains(got, "inspect_code") {
		t.Fatalf("tool discipline exposes legacy inspect_code:\n%s", got)
	}
}

func TestToolDisciplineUsesCompactCapabilityActions(t *testing.T) {
	got := Render(Spec{})
	for _, want := range []string{
		"git action=status",
		"edit action=replace",
		"patch for bounded multi-file changes",
		"write for complete file creation or replacement",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("tool discipline missing compact capability guidance %q:\n%s", want, got)
		}
	}
}
