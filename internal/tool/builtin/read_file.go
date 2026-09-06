package builtin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

const maxReadFileBytes = 2 * 1024 * 1024

type readFileHandler struct {
	workspace *workspace.Workspace
}

type readFileInput struct {
	Path         string `json:"path"`
	Offset       int64  `json:"offset,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
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
		Mutability:          tool.MutabilityReadOnly,
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
				"continuation": map[string]any{
					"type":        "string",
					"description": "Snapshot token from a truncated result; send it with next_offset to detect file changes",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxReadFileBytes,
					"description": "Target page size in bytes; defaults to 2 MiB and may extend to finish one UTF-8 code point",
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
	continuation, err := continuationToken("read_file", struct {
		Path string `json:"path"`
	}{Path: input.Path}, fileSnapshot(fileInfo))
	if err != nil {
		_ = file.Close()
		return tool.Result{}, err
	}
	if input.Continuation != "" && input.Continuation != continuation {
		_ = file.Close()
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeStaleContinuation, "read_file continuation is stale; restart from offset 0")
	}
	if input.Offset > size {
		input.Offset = size
	}
	if input.Offset > 0 && input.Offset < size {
		var boundary [1]byte
		if _, err := file.ReadAt(boundary[:], input.Offset); err != nil {
			_ = file.Close()
			return tool.Result{}, fmt.Errorf("inspect %q at byte %d: %w", input.Path, input.Offset, err)
		}
		if !utf8.RuneStart(boundary[0]) {
			_ = file.Close()
			return tool.Result{}, tool.NewToolError(
				tool.ErrorCodeInvalidArguments,
				fmt.Sprintf("read_file offset %d splits a UTF-8 code point; use next_offset from the previous page", input.Offset),
			)
		}
	}
	if input.Offset > 0 {
		if _, err := file.Seek(input.Offset, io.SeekStart); err != nil {
			_ = file.Close()
			return tool.Result{}, fmt.Errorf("seek %q to byte %d: %w", input.Path, input.Offset, err)
		}
	}

	remaining := size - input.Offset
	truncated := remaining > int64(input.Limit)
	readBytes := remaining
	if truncated {
		readBytes = int64(input.Limit + utf8.UTFMax - 1)
		if readBytes > remaining {
			readBytes = remaining
		}
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

	if truncated {
		cut, err := utf8PageCut(contents, input.Limit)
		if err != nil {
			return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q is not valid UTF-8 near byte %d", input.Path, input.Offset))
		}
		contents = contents[:cut]
	} else if !utf8.Valid(contents) {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q is not valid UTF-8", input.Path))
	}

	var nextOffset *int64
	if truncated {
		next := input.Offset + int64(len(contents))
		nextOffset = &next
	}
	output := string(contents)
	if truncated {
		output += fmt.Sprintf("\n[output truncated; continue with offset=%d]", *nextOffset)
	}

	var contentSHA256 string
	if input.Offset == 0 && !truncated {
		digest := sha256.Sum256(contents)
		contentSHA256 = fmt.Sprintf("%x", digest[:])
	}
	return tool.Result{
		CallID:     call.ID,
		ToolName:   call.Name,
		Output:     output,
		SHA256:     contentSHA256,
		Truncated:  truncated,
		NextOffset: nextOffset,
		Continuation: func() string {
			if truncated {
				return continuation
			}
			return ""
		}(),
	}, nil
}
func utf8PageCut(data []byte, target int) (int, error) {
	if target <= 0 || len(data) == 0 {
		return 0, nil
	}
	position := 0
	for position < len(data) {
		_, size := utf8.DecodeRune(data[position:])
		if size == 1 && data[position] >= utf8.RuneSelf {
			return 0, fmt.Errorf("invalid UTF-8 at page byte %d", position)
		}
		next := position + size
		if next > target {
			if position == 0 {
				return next, nil
			}
			return position, nil
		}
		position = next
		if position == target {
			return position, nil
		}
	}
	return position, nil
}
