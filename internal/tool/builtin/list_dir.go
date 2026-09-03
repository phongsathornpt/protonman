package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

const maxDirectoryEntries = 1000

type listDirHandler struct {
	workspace *workspace.Workspace
}

type listDirInput struct {
	Path string `json:"path"`
}

// NewListDir returns the protected-aware directory listing adapter.
func NewListDir(workspaceRoot *workspace.Workspace) tool.Handler {
	return listDirHandler{workspace: workspaceRoot}
}

func (listDirHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "list_dir",
		Description:         "List entries in a workspace directory.",
		Kind:                tool.KindRead,
		PermissionDetailKey: "path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path, defaulting to the workspace root",
				},
			},
		},
	}
}

func (h listDirHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("list_dir workspace is required")
	}
	var input listDirInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode list_dir arguments: %w", err)
	}
	if strings.TrimSpace(input.Path) == "" {
		input.Path = "."
	}
	resolvedPath, err := h.workspace.Resolve(ctx, input.Path)
	if err != nil {
		return tool.Result{}, err
	}
	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return tool.Result{}, fmt.Errorf("list %q: %w", input.Path, err)
	}
	var output strings.Builder
	output.Grow(len(entries) * 40)
	truncated := false
	for index, entry := range entries {
		if err := ctx.Err(); err != nil {
			return tool.Result{}, fmt.Errorf("list %q: %w", input.Path, err)
		}
		if index >= maxDirectoryEntries {
			truncated = true
			break
		}
		childPath := filepath.Join(resolvedPath, entry.Name())
		if h.workspace.IsProtected(childPath) {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if err := h.workspace.CheckAbsolute(ctx, childPath); err != nil {
				continue
			}
			output.WriteString("link ")
			output.WriteString(entry.Name())
			output.WriteByte('\n')
			continue
		}
		if entry.IsDir() {
			output.WriteString("dir  ")
			output.WriteString(entry.Name())
			output.WriteByte('/')
			output.WriteByte('\n')
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return tool.Result{}, fmt.Errorf("stat directory entry %q: %w", entry.Name(), err)
		}
		output.WriteString("file ")
		output.WriteString(entry.Name())
		output.WriteString(" (")
		output.WriteString(strconv.FormatInt(info.Size(), 10))
		output.WriteString(" bytes)\n")
	}
	if truncated {
		output.WriteString("[directory output truncated at 1000 entries]\n")
	}
	return tool.Result{
		CallID:    call.ID,
		ToolName:  call.Name,
		Output:    output.String(),
		Truncated: truncated,
	}, nil
}
