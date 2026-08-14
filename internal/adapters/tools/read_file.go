package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/projectTHORN/proton/internal/domain/tool"
)

type readFileHandler struct{}

type readFileInput struct {
	Path string `json:"path"`
}

// NewReadFile returns the filesystem read adapter.
func NewReadFile() tool.Handler {
	return readFileHandler{}
}

func (readFileHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "read_file",
		Description:         "Read a UTF-8 text file from the current workspace.",
		Kind:                tool.KindRead,
		PermissionDetailKey: "path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the file to read",
				},
			},
			"required": []string{"path"},
		},
	}
}

func (readFileHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	var input readFileInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode read_file arguments: %w", err)
	}
	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		return tool.Result{}, fmt.Errorf("read_file path is required")
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{}, fmt.Errorf("before reading %q: %w", input.Path, err)
	}

	contents, err := os.ReadFile(input.Path)
	if err != nil {
		return tool.Result{}, fmt.Errorf("read %q: %w", input.Path, err)
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{}, fmt.Errorf("after reading %q: %w", input.Path, err)
	}

	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   string(contents),
	}, nil
}
