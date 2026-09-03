package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/projectTHORN/proton/internal/workspace"
	"github.com/projectTHORN/proton/internal/tool"
)

const maxReadFileBytes = 2 * 1024 * 1024

type readFileHandler struct {
	workspace *workspace.Workspace
}

type readFileInput struct {
	Path string `json:"path"`
}

// NewReadFile returns the filesystem read adapter.
func NewReadFile(workspaceRoot *workspace.Workspace) tool.Handler {
	return readFileHandler{workspace: workspaceRoot}
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

func (h readFileHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("read_file workspace is required")
	}
	var input readFileInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode read_file arguments: %w", err)
	}
	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		return tool.Result{}, fmt.Errorf("read_file path is required")
	}
	path, err := h.workspace.Resolve(ctx, input.Path)
	if err != nil {
		return tool.Result{}, err
	}

	file, err := os.Open(path)
	if err != nil {
		return tool.Result{}, fmt.Errorf("open %q: %w", input.Path, err)
	}
	fileInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return tool.Result{}, fmt.Errorf("stat %q: %w", input.Path, err)
	}

	size := fileInfo.Size()
	var contents []byte
	var truncated bool

	if size > 0 && size <= maxReadFileBytes {
		contents = make([]byte, size)
		_, readErr := io.ReadFull(file, contents)
		closeErr := file.Close()
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return tool.Result{}, fmt.Errorf("read %q: %w", input.Path, readErr)
		}
		if closeErr != nil {
			return tool.Result{}, fmt.Errorf("close %q: %w", input.Path, closeErr)
		}
	} else {
		contents, err = io.ReadAll(io.LimitReader(file, maxReadFileBytes+1))
		closeErr := file.Close()
		if err != nil {
			return tool.Result{}, fmt.Errorf("read %q: %w", input.Path, err)
		}
		if closeErr != nil {
			return tool.Result{}, fmt.Errorf("close %q: %w", input.Path, closeErr)
		}
		if len(contents) > maxReadFileBytes {
			truncated = true
			contents = contents[:maxReadFileBytes]
		}
	}
	output := string(contents)
	if truncated {
		output += "\n[output truncated at 2 MiB]"
	}

	return tool.Result{
		CallID:    call.ID,
		ToolName:  call.Name,
		Output:    output,
		Truncated: truncated,
	}, nil
}
