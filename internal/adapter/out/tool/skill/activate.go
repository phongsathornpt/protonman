package skilltool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
)

type activateSkillHandler struct {
	registry  *skill.Registry
	workspace *workspace.Workspace
}

type activateSkillInput struct {
	Name string `json:"name"`
}

// NewActivateSkill creates a tool.Handler that activates an Agent Skill.
func NewActivateSkill(registry *skill.Registry, workspaceRoots ...*workspace.Workspace) tool.Handler {
	var ws *workspace.Workspace
	if len(workspaceRoots) > 0 {
		ws = workspaceRoots[0]
	}
	return activateSkillHandler{
		registry:  registry,
		workspace: ws,
	}
}

// BindSkillRegistry clones this handler for an isolated subagent skill session.
func (h activateSkillHandler) BindSkillRegistry(registry *skill.Registry) tool.Handler {
	h.registry = registry
	return h
}

func (activateSkillHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                tool.NameSkill,
		Description:         "Activate a specialized skill. Its full instructions load into the next model context.",
		Kind:                tool.KindForName("skill"),
		Mutability:          tool.MutabilityMutating,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainWorkspacePolicy, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyExternalRead},
		PermissionDetailKey: "name",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the skill to activate (e.g. 'pdf-processing')",
				},
			},
			"required":             []string{"name"},
			"additionalProperties": false,
		},
	}
}

func (h activateSkillHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.registry == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "skill registry is not configured")
	}
	var input activateSkillInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode skill arguments", err)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "skill name is required")
	}

	s, ok := h.registry.Lookup(name)
	if !ok {
		available := make([]string, 0)
		for _, item := range h.registry.Catalog() {
			available = append(available, item.Name)
		}
		msg := fmt.Sprintf("skill %q not found", name)
		if len(available) > 0 {
			msg = fmt.Sprintf("skill %q not found; available skills: %s", name, strings.Join(available, ", "))
		}
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeNotFound, msg)
	}

	if h.workspace != nil && s.BaseDir != "" {
		if err := h.workspace.AddReadRoot(s.BaseDir); err != nil {
			return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "authorize skill directory", err)
		}
	}

	if err := h.registry.Activate(name); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeOutputTooLarge, "activate skill context", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Activated skill %q; full instructions are loaded into the next model context.", s.Name)
	if s.BaseDir != "" {
		fmt.Fprintf(&b, "\nSkill directory: %s", s.BaseDir)
	}
	if len(s.Resources) > 0 {
		b.WriteString("\nResources:")
		for _, resource := range s.Resources {
			fmt.Fprintf(&b, "\n- %s", resource)
		}
	}
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: b.String()}, nil
}
