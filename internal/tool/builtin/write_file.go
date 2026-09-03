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

type writeFileHandler struct {
	workspace   *workspace.Workspace
	checkpoints checkpoint.Store
}

type writeFileInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// NewWriteFile returns the atomic whole-file write adapter.
func NewWriteFile(workspaceRoot *workspace.Workspace, stores ...checkpoint.Store) tool.Handler {
	return writeFileHandler{
		workspace:   workspaceRoot,
		checkpoints: selectCheckpointStore(stores),
	}
}

func (writeFileHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "write_file",
		Description:         "Create or replace a UTF-8 text file atomically.",
		Kind:                tool.KindEdit,
		PermissionDetailKey: "file_path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{"type": "string"},
				"content":   map[string]any{"type": "string"},
			},
			"required": []string{"file_path", "content"},
		},
	}
}

func (h writeFileHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("write_file workspace is required")
	}
	var input writeFileInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode write_file arguments: %w", err)
	}
	input.FilePath = strings.TrimSpace(input.FilePath)
	if input.FilePath == "" {
		return tool.Result{}, fmt.Errorf("write_file file_path is required")
	}
	resolvedPath, err := h.workspace.Resolve(ctx, input.FilePath)
	if err != nil {
		return tool.Result{}, err
	}
	checkpointID, err := h.checkpoints.Capture(ctx, []string{resolvedPath})
	if err != nil {
		return tool.Result{}, fmt.Errorf("checkpoint %q: %w", input.FilePath, err)
	}
	if err := atomicWrite(ctx, h.workspace, resolvedPath, []byte(input.Content)); err != nil {
		return tool.Result{
			CallID:       call.ID,
			ToolName:     call.Name,
			CheckpointID: checkpointID,
		}, fmt.Errorf("write %q: %w", input.FilePath, err)
	}
	return tool.Result{
		CallID:       call.ID,
		ToolName:     call.Name,
		Output:       fmt.Sprintf("Wrote file successfully to %s.", resolvedPath),
		CheckpointID: checkpointID,
	}, nil
}
