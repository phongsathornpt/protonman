package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/projectTHORN/proton/internal/checkpoint"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
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

func (searchReplaceHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "search_replace",
		Description:         "Replace an exact string in a workspace file.",
		Kind:                tool.KindEdit,
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
			"required": []string{"file_path", "old_string", "new_string"},
		},
	}
}

func (h searchReplaceHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("search_replace workspace is required")
	}
	var input searchReplaceInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode search_replace arguments: %w", err)
	}
	input.FilePath = strings.TrimSpace(input.FilePath)
	if input.FilePath == "" {
		return tool.Result{}, fmt.Errorf("search_replace file_path is required")
	}
	if input.OldString == input.NewString {
		return tool.Result{}, fmt.Errorf("search_replace old_string and new_string must differ")
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
		checkpointID, err := h.checkpoints.Capture(ctx, []string{resolvedPath})
		if err != nil {
			return tool.Result{}, fmt.Errorf("checkpoint %q: %w", input.FilePath, err)
		}
		if err := atomicWrite(ctx, h.workspace, resolvedPath, []byte(input.NewString)); err != nil {
			return tool.Result{
				CallID:       call.ID,
				ToolName:     call.Name,
				CheckpointID: checkpointID,
			}, fmt.Errorf("create %q: %w", input.FilePath, err)
		}
		return editResult(call, resolvedPath, "created", checkpointID)
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
	checkpointID, err := h.checkpoints.Capture(ctx, []string{resolvedPath})
	if err != nil {
		return tool.Result{}, fmt.Errorf("checkpoint %q: %w", input.FilePath, err)
	}
	if err := atomicWrite(ctx, h.workspace, resolvedPath, []byte(updated)); err != nil {
		return tool.Result{
			CallID:       call.ID,
			ToolName:     call.Name,
			CheckpointID: checkpointID,
		}, fmt.Errorf("update %q: %w", input.FilePath, err)
	}
	return editResult(call, resolvedPath, "updated", checkpointID)
}

func editResult(call tool.Call, path string, action string, checkpointID string) (tool.Result, error) {
	return tool.Result{
		CallID:       call.ID,
		ToolName:     call.Name,
		Output:       fmt.Sprintf("The file %s has been %s.", path, action),
		CheckpointID: checkpointID,
	}, nil
}
