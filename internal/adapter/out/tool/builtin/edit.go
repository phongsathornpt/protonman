package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	"github.com/phongsathornpt/protonman/internal/platform/checkpoint"
)

type editHandler struct {
	write   tool.Handler
	replace tool.Handler
	patch   tool.Handler
	restore tool.Handler
}

type editInput struct {
	Action         string `json:"action"`
	FilePath       string `json:"file_path,omitempty"`
	Content        string `json:"content,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
	OldString      string `json:"old_string,omitempty"`
	NewString      string `json:"new_string,omitempty"`
	ReplaceAll     bool   `json:"replace_all,omitempty"`
	Patch          string `json:"patch,omitempty"`
	CheckpointID   string `json:"checkpoint_id,omitempty"`
}

func NewEdit(workspaceRoot *workspace.Workspace, store checkpoint.Store) tool.Handler {
	return editHandler{
		write:   NewWriteFile(workspaceRoot, store),
		replace: NewSearchReplace(workspaceRoot, store),
		patch:   NewApplyPatch(workspaceRoot, store),
		restore: NewCheckpointRestore(store),
	}
}

func (h editHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:        tool.NameEdit,
		Description: "Edit workspace files. Use action=write to create/replace a full file, replace for exact text replacement, patch for a bounded multi-file patch, or restore for a Protonman checkpoint.",
		Kind:        tool.KindEdit,
		Mutability:  tool.MutabilityMutating,
		Safety:      tool.SafetyContract{MutationDomain: tool.MutationDomainWorkspace, MutationSafety: tool.MutationSafetyDynamic, CheckpointPolicy: tool.CheckpointPolicyWhenKnown, Boundary: tool.BoundaryPolicyWorkspaceWrite},
		Semantics:   h.callSemantics,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":          map[string]any{"type": "string", "enum": []string{"write", "replace", "patch", "restore"}, "description": "Edit operation to perform"},
				"file_path":       map[string]any{"type": "string", "description": "Workspace-relative file path for write or replace"},
				"content":         map[string]any{"type": "string", "description": "Complete UTF-8 file content for write"},
				"expected_sha256": map[string]any{"type": "string", "description": "SHA-256 from a complete read result when overwriting an existing file"},
				"old_string":      map[string]any{"type": "string", "description": "Exact text to replace"},
				"new_string":      map[string]any{"type": "string", "description": "Replacement text"},
				"replace_all":     map[string]any{"type": "boolean", "description": "Replace all exact matches"},
				"patch":           map[string]any{"type": "string", "description": "Patch enclosed by *** Begin Patch and *** End Patch"},
				"checkpoint_id":   map[string]any{"type": "string", "description": "Checkpoint identifier to restore"},
			},
			"required":             []string{"action"},
			"additionalProperties": false,
		},
	}
}

func (h editHandler) child(action string) (tool.Handler, bool) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "write":
		return h.write, true
	case "replace":
		return h.replace, true
	case "patch":
		return h.patch, true
	case "restore":
		return h.restore, true
	default:
		return nil, false
	}
}

func (h editHandler) callSemantics(arguments json.RawMessage) tool.CallSemantics {
	fallback := tool.StaticCallSemantics(h.staticDefinition())
	var input struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(arguments, &input) != nil {
		return fallback
	}
	child, ok := h.child(input.Action)
	if !ok {
		return fallback
	}
	definition := child.Definition()
	return tool.StaticCallSemantics(definition)
}

func (h editHandler) staticDefinition() tool.Definition {
	return tool.Definition{
		Kind:       tool.KindEdit,
		Mutability: tool.MutabilityMutating,
		Safety:     tool.SafetyContract{MutationDomain: tool.MutationDomainWorkspace, MutationSafety: tool.MutationSafetyDynamic, CheckpointPolicy: tool.CheckpointPolicyWhenKnown, Boundary: tool.BoundaryPolicyWorkspaceWrite},
	}
}

func (h editHandler) PermissionDetail(arguments json.RawMessage) string {
	action, childArgs, child, err := h.resolve(arguments)
	if err != nil {
		return ""
	}
	if provider, ok := child.(tool.DetailProvider); ok {
		if detail := strings.TrimSpace(provider.PermissionDetail(childArgs)); detail != "" {
			return detail
		}
	}
	var input editInput
	_ = json.Unmarshal(arguments, &input)
	switch action {
	case "write", "replace":
		return strings.TrimSpace(input.FilePath)
	case "restore":
		return strings.TrimSpace(input.CheckpointID)
	default:
		return ""
	}
}

func (h editHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	_, childArgs, child, err := h.resolve(call.Arguments)
	if err != nil {
		return tool.Result{}, err
	}
	childCall := call
	childCall.Arguments = tool.NormalizeArguments(child.Definition(), childArgs)
	result, err := child.Execute(ctx, childCall)
	if result.ToolName == "" {
		result.ToolName = call.Name
	}
	return result, err
}

func (h editHandler) resolve(arguments json.RawMessage) (string, json.RawMessage, tool.Handler, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &object); err != nil {
		return "", nil, nil, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode edit arguments", err)
	}
	var action string
	if raw, ok := object["action"]; ok {
		_ = json.Unmarshal(raw, &action)
	}
	action = strings.ToLower(strings.TrimSpace(action))
	child, ok := h.child(action)
	if !ok {
		return "", nil, nil, tool.NewToolError(tool.ErrorCodeInvalidArguments, "edit action must be write, replace, patch, or restore")
	}
	delete(object, "action")
	childArgs, err := json.Marshal(object)
	if err != nil {
		return "", nil, nil, fmt.Errorf("encode edit %s arguments: %w", action, err)
	}
	return action, childArgs, child, nil
}
