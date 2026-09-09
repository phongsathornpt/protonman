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

type searchReplaceHandler struct {
	workspace   *workspace.Workspace
	checkpoints checkpoint.Store
}

type searchReplaceInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

// NewSearchReplace returns the exact search-and-replace edit adapter.
func NewSearchReplace(workspaceRoot *workspace.Workspace, stores ...checkpoint.Store) tool.Handler {
	return searchReplaceHandler{
		workspace:   workspaceRoot,
		checkpoints: selectCheckpointStore(stores),
	}
}

func (h searchReplaceHandler) PermissionDetail(arguments json.RawMessage) string {
	var input searchReplaceInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return ""
	}
	return managedFileDetail(input.FilePath, nil)
}

func (searchReplaceHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "edit",
		Description:         "Replace an exact string in a workspace file.",
		Kind:                tool.KindEdit,
		Mutability:          tool.MutabilityMutating,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainWorkspace, MutationSafety: tool.MutationSafetyContextual, CheckpointPolicy: tool.CheckpointPolicyRequired, Boundary: tool.BoundaryPolicyWorkspaceWrite},
		PermissionDetailKey: "file_path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path":  map[string]any{"type": "string"},
				"old_string": map[string]any{"type": "string"},
				"new_string": map[string]any{"type": "string"},
				"replace_all": map[string]any{
					"type":    "boolean",
					"default": false,
				},
			},
			"required":             []string{"file_path", "old_string", "new_string"},
			"additionalProperties": false,
		},
	}
}

func (h searchReplaceHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("edit replace workspace is required")
	}
	var input searchReplaceInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode edit replace arguments", err)
	}
	input.FilePath = strings.TrimSpace(input.FilePath)
	if input.FilePath == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "edit replace file_path is required")
	}
	if input.OldString == input.NewString {
		return tool.Result{}, fmt.Errorf("edit replace old_string and new_string must differ")
	}
	resolvedPath, err := h.workspace.Resolve(ctx, input.FilePath)
	if err != nil {
		return tool.Result{}, err
	}
	contents, exists, err := readEditFile(ctx, h.workspace, resolvedPath)
	if err != nil {
		return tool.Result{}, fmt.Errorf("read %q: %w", input.FilePath, err)
	}
	if input.OldString == "" {
		if exists && len(contents) > 0 {
			return tool.Result{}, fmt.Errorf("empty old_string cannot overwrite a non-empty file")
		}
		checkpointID, err := prepareWorkspaceMutation(ctx, h.workspace, h.checkpoints, h.Definition().Safety, nil, []string{resolvedPath})
		if err != nil {
			return tool.Result{}, err
		}
		displayPath := input.FilePath
		if rel, relErr := h.workspace.RelRead(resolvedPath); relErr == nil && rel != "" {
			displayPath = rel
		}
		if err := atomicWriteResolved(ctx, h.workspace, resolvedPath, []byte(input.NewString)); err != nil {
			return tool.Result{
				CallID:       call.ID,
				ToolName:     call.Name,
				CheckpointID: checkpointID,
			}, fmt.Errorf("create %q: %w", input.FilePath, err)
		}
		h.workspace.MarkMutationOwned(ctx, resolvedPath)
		return editResult(call, displayPath, "created", checkpointID)
	}
	if !exists {
		return tool.Result{}, fmt.Errorf("edit target %q does not exist", input.FilePath)
	}

	content := string(contents)
	occurrences := strings.Count(content, input.OldString)
	if occurrences == 0 {
		return tool.Result{}, fmt.Errorf("old_string was not found in %q", input.FilePath)
	}
	if occurrences > 1 && !input.ReplaceAll {
		return tool.Result{}, fmt.Errorf("old_string matched %d locations; use replace_all for multiple matches", occurrences)
	}
	updated := strings.Replace(content, input.OldString, input.NewString, 1)
	if input.ReplaceAll {
		updated = strings.ReplaceAll(content, input.OldString, input.NewString)
	}
	checkpointID, err := prepareWorkspaceMutation(ctx, h.workspace, h.checkpoints, h.Definition().Safety, nil, []string{resolvedPath})
	if err != nil {
		return tool.Result{}, err
	}
	displayPath := input.FilePath
	if rel, relErr := h.workspace.RelRead(resolvedPath); relErr == nil && rel != "" {
		displayPath = rel
	}
	if err := atomicWriteResolved(ctx, h.workspace, resolvedPath, []byte(updated)); err != nil {
		return tool.Result{
			CallID:       call.ID,
			ToolName:     call.Name,
			CheckpointID: checkpointID,
		}, fmt.Errorf("update %q: %w", input.FilePath, err)
	}
	h.workspace.MarkMutationOwned(ctx, resolvedPath)
	return editResult(call, displayPath, "updated", checkpointID)
}

func editResult(call tool.Call, path string, action string, checkpointID string) (tool.Result, error) {
	return tool.Result{
		CallID:           call.ID,
		ToolName:         call.Name,
		Output:           fmt.Sprintf("The file %s has been %s.", path, action),
		CheckpointID:     checkpointID,
		MutationCoverage: tool.MutationCoverageFull,
		AffectedPaths:    []string{path},
	}, nil
}
