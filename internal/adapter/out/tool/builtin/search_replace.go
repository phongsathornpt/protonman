package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/base/diffutil"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	"github.com/phongsathornpt/protonman/internal/platform/checkpoint"
)

type searchReplaceHandler struct {
	workspace   *workspace.Workspace
	checkpoints checkpoint.Store
}

type searchReplaceInput struct {
	FilePath   string `json:"filePath"`
	OldString  string `json:"oldString"`
	NewString  string `json:"newString"`
	ReplaceAll bool   `json:"replaceAll"`
}

func (in *searchReplaceInput) UnmarshalJSON(data []byte) error {
	type alias searchReplaceInput
	var aux struct {
		alias
		PathAlias string `json:"path"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*in = searchReplaceInput(aux.alias)
	if in.FilePath == "" {
		in.FilePath = aux.PathAlias
	}
	return nil
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
		Name:                tool.NameEdit,
		Description:         "Replace an exact string in a workspace file.",
		Kind:                tool.KindEdit,
		Mutability:          tool.MutabilityMutating,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainWorkspace, MutationSafety: tool.MutationSafetyContextual, CheckpointPolicy: tool.CheckpointPolicyRequired, Boundary: tool.BoundaryPolicyWorkspaceWrite},
		PermissionDetailKey: "filePath",
		InputAliases: map[string][]string{
			"filePath": {"path", "filepath"},
		},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filePath":  map[string]any{"type": "string"},
				"oldString": map[string]any{"type": "string"},
				"newString": map[string]any{"type": "string"},
				"replaceAll": map[string]any{
					"type":    "boolean",
					"default": false,
				},
			},
			"required":             []string{"filePath", "oldString", "newString"},
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
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "edit replace filePath is required")
	}
	if input.OldString == input.NewString {
		return tool.Result{}, fmt.Errorf("edit replace oldString and newString must differ")
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
		diff := diffutil.NewFileDiff(displayPath, input.NewString, 3)
		return editResult(call, displayPath, "created", checkpointID, diff)
	}
	if !exists {
		return tool.Result{}, fmt.Errorf("edit target %q does not exist", input.FilePath)
	}

	content := string(contents)
	matches, err := findReplacements(content, input.OldString, input.NewString, input.ReplaceAll, input.FilePath)
	if err != nil {
		return tool.Result{}, err
	}
	updated := applyReplacements(content, matches)
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
	diff := diffutil.UnifiedDiff(content, updated, displayPath, 3)
	return editResult(call, displayPath, "updated", checkpointID, diff)
}

func editResult(call tool.Call, path string, action string, checkpointID string, diff string) (tool.Result, error) {
	adds, dels := diffutil.DiffStats(diff)
	payload := map[string]any{
		"path":      path,
		"action":    action,
		"additions": adds,
		"deletions": dels,
		"diff":      diff,
	}
	structured, _ := json.Marshal(payload)
	return tool.Result{
		CallID:           call.ID,
		ToolName:         call.Name,
		Output:           fmt.Sprintf("The file %s has been %s.", path, action),
		StructuredOutput: structured,
		CheckpointID:     checkpointID,
		MutationCoverage: tool.MutationCoverageFull,
		AffectedPaths:    []string{path},
	}, nil
}
