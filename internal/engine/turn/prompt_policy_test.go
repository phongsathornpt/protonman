package turn

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
)

func TestEffectivePromptSpecDerivesCapabilitiesAndMutationDomains(t *testing.T) {
	loop := &Loop{promptSpec: &prompt.Spec{}, languageModel: &scriptedClient{}}
	defs := []tool.Definition{
		{Name: "tasks", Kind: tool.KindTask, Mutability: tool.MutabilityMutating, Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainTaskState}},
		{Name: "agents", Kind: tool.KindAgent, Mutability: tool.MutabilityMutating, Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainAgentState}},
		{Name: "edit", Kind: tool.KindEdit, Mutability: tool.MutabilityMutating, Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainWorkspace}},
		{Name: "mcp.read", Kind: tool.KindMCP, Mutability: tool.MutabilityReadOnly},
		{Name: "mcp.write", Kind: tool.KindMCP, Mutability: tool.MutabilityMutating},
	}
	got := loop.effectivePromptSpec(defs, nil)
	if !got.Capabilities.Tasks || !got.Capabilities.Agents || !got.Capabilities.MCP {
		t.Fatalf("capabilities = %+v", got.Capabilities)
	}
	if joined := strings.Join(got.AvailableTools, ","); joined != "tasks,agents,edit,mcp.read,mcp.write" {
		t.Fatalf("available tools = %q", joined)
	}
	if !got.Mutations.Task || !got.Mutations.Agent || !got.Mutations.Workspace || !got.Mutations.External {
		t.Fatalf("mutations = %+v", got.Mutations)
	}
}

func TestNonWorkspaceMutationDoesNotEnableEditingVerificationPrompt(t *testing.T) {
	loop := &Loop{promptSpec: &prompt.Spec{}, languageModel: &scriptedClient{}}
	defs := []tool.Definition{{Name: "agents", Kind: tool.KindAgent, Mutability: tool.MutabilityMutating, Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainAgentState}}}
	got := prompt.Render(loop.effectivePromptSpec(defs, nil))
	if strings.Contains(got, "# Editing And Verification") {
		t.Fatalf("agent-state mutation enabled workspace verification contract:\n%s", got)
	}
}
