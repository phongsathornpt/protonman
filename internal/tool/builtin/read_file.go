package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

const maxReadFileBytes = 2 * 1024 * 1024

type readFileHandler struct {
	workspace *workspace.Workspace
}

type readFileInput struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
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
				"offset": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Byte offset to start reading from; use next_offset from a truncated result",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxReadFileBytes,
					"description": "Maximum bytes to return; defaults to 2 MiB",
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
	if input.Offset < 0 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read_file offset must be non-negative")
	}
	if input.Limit < 0 || input.Limit > maxReadFileBytes {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read_file limit must be between 1 byte and 2 MiB")
	}
	if input.Limit == 0 {
		input.Limit = maxReadFileBytes
	}
	path, err := h.workspace.ResolveRead(ctx, input.Path)
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
	if fileInfo.IsDir() {
		_ = file.Close()
		return tool.Result{}, tool.NewToolError(
			tool.ErrorCodeInvalidArguments,
			fmt.Sprintf("%q is a directory; use list_dir instead", input.Path),
		)
	}
	if !fileInfo.Mode().IsRegular() {
		_ = file.Close()
		return tool.Result{}, tool.NewToolError(
			tool.ErrorCodeInvalidArguments,
			fmt.Sprintf("%q is not a regular file", input.Path),
		)
	}

	size := fileInfo.Size()
	if input.Offset > 0 {
		if _, err := file.Seek(input.Offset, io.SeekStart); err != nil {
			_ = file.Close()
			return tool.Result{}, fmt.Errorf("seek %q to byte %d: %w", input.Path, input.Offset, err)
		}
	}

	remaining := size - input.Offset
	if remaining < 0 {
		remaining = 0
	}
	readBytes := remaining
	if readBytes > int64(input.Limit) {
		readBytes = int64(input.Limit)
	}
	contents := make([]byte, int(readBytes))
	_, readErr := io.ReadFull(file, contents)
	closeErr := file.Close()
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		return tool.Result{}, fmt.Errorf("read %q: %w", input.Path, readErr)
	}
	if closeErr != nil {
		return tool.Result{}, fmt.Errorf("close %q: %w", input.Path, closeErr)
	}

	truncated := remaining > int64(input.Limit)
	var nextOffset *int64
	if truncated {
		next := input.Offset + int64(len(contents))
		nextOffset = &next
	}
	output := string(contents)
	if truncated {
		output += fmt.Sprintf("\n[output truncated; continue with offset=%d]", *nextOffset)
	}

	return tool.Result{
		CallID:     call.ID,
		ToolName:   call.Name,
		Output:     output,
		Truncated:  truncated,
		NextOffset: nextOffset,
	}, nil
}
