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
	Path      string `json:"path"`
	DirPath   string `json:"dir_path"`
	Directory string `json:"directory"`
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
				"dir_path": map[string]any{
					"type":        "string",
					"description": "Alias for path",
				},
				"directory": map[string]any{
					"type":        "string",
					"description": "Alias for path",
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

	targetPath := strings.TrimSpace(input.Path)
	if targetPath == "" {
		targetPath = strings.TrimSpace(input.DirPath)
	}
	if targetPath == "" {
		targetPath = strings.TrimSpace(input.Directory)
	}
	if targetPath == "" {
		targetPath = "."
	}

	resolvedPath, err := h.workspace.Resolve(ctx, targetPath)
	if err != nil {
		return tool.Result{}, err
	}
	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return tool.Result{}, fmt.Errorf("list %q: %w", targetPath, err)
	}

	allocHint := len(entries)
	if allocHint > maxDirectoryEntries {
		allocHint = maxDirectoryEntries
	}
	var output strings.Builder
	output.Grow(allocHint * 48)

	emitted := 0
	truncated := false

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return tool.Result{}, fmt.Errorf("list %q: %w", targetPath, err)
		}
		if emitted >= maxDirectoryEntries {
			truncated = true
			break
		}
		childPath := filepath.Join(resolvedPath, entry.Name())
		if h.workspace.IsProtected(childPath) {
			continue
		}

		entryType := entry.Type()

		// Symlink handling with destination and directory indicator
		if entryType&os.ModeSymlink != 0 {
			if err := h.workspace.CheckAbsolute(ctx, childPath); err != nil {
				continue
			}
			target, readlinkErr := os.Readlink(childPath)
			targetInfo, statErr := os.Stat(childPath)
			isDir := statErr == nil && targetInfo.IsDir()

			output.WriteString("link ")
			output.WriteString(entry.Name())
			if isDir {
				output.WriteByte('/')
			}
			if readlinkErr == nil && target != "" {
				output.WriteString(" -> ")
				output.WriteString(target)
				if isDir && !strings.HasSuffix(target, "/") {
					output.WriteByte('/')
				}
			}
			output.WriteByte('\n')
			emitted++
			continue
		}

		// Directory handling
		if entry.IsDir() {
			output.WriteString("dir  ")
			output.WriteString(entry.Name())
			output.WriteByte('/')
			output.WriteByte('\n')
			emitted++
			continue
		}

		// Special files
		if entryType&os.ModeSocket != 0 {
			output.WriteString("sock ")
			output.WriteString(entry.Name())
			output.WriteByte('\n')
			emitted++
			continue
		}
		if entryType&os.ModeNamedPipe != 0 {
			output.WriteString("fifo ")
			output.WriteString(entry.Name())
			output.WriteByte('\n')
			emitted++
			continue
		}
		if entryType&os.ModeDevice != 0 {
			output.WriteString("dev  ")
			output.WriteString(entry.Name())
			output.WriteByte('\n')
			emitted++
			continue
		}

		// Regular file handling with stat error resilience
		info, err := entry.Info()
		if err != nil {
			// Resilient fallback: don't abort the entire directory listing on a single transient error
			output.WriteString("file ")
			output.WriteString(entry.Name())
			output.WriteString(" (unknown size)\n")
			emitted++
			continue
		}

		output.WriteString("file ")
		output.WriteString(entry.Name())
		output.WriteString(" (")
		output.WriteString(formatFileSize(info.Size()))
		output.WriteString(")\n")
		emitted++
	}

	if emitted == 0 && !truncated {
		output.WriteString("(empty directory)\n")
	} else if truncated {
		output.WriteString(fmt.Sprintf("[directory output truncated at %d entries]\n", maxDirectoryEntries))
	}

	return tool.Result{
		CallID:    call.ID,
		ToolName:  call.Name,
		Output:    output.String(),
		Truncated: truncated,
	}, nil
}

func formatFileSize(bytes int64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)
	switch {
	case bytes >= gib:
		return fmt.Sprintf("%.1f GiB, %s bytes", float64(bytes)/float64(gib), strconv.FormatInt(bytes, 10))
	case bytes >= mib:
		return fmt.Sprintf("%.1f MiB, %s bytes", float64(bytes)/float64(mib), strconv.FormatInt(bytes, 10))
	case bytes >= kib:
		return fmt.Sprintf("%.1f KiB, %s bytes", float64(bytes)/float64(kib), strconv.FormatInt(bytes, 10))
	default:
		return strconv.FormatInt(bytes, 10) + " bytes"
	}
}
