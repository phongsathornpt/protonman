package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/core/workspace"
)

const maxDirectoryEntries = 1000

type listDirHandler struct {
	workspace *workspace.Workspace
}

type listDirInput struct {
	Path         string `json:"path"`
	Offset       int    `json:"offset,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

// NewListDir returns the protected-aware directory listing adapter.
func NewListDir(workspaceRoot *workspace.Workspace) tool.Handler {
	return listDirHandler{workspace: workspaceRoot}
}

func (listDirHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "list_dir",
		Description:         "List entries in a workspace directory. Prefer this over shell ls for directory inspection.",
		Kind:                tool.KindForName("list_dir"),
		Mutability:          tool.MutabilityReadOnly,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyWorkspaceRead},
		Evidence:            tool.EvidenceWorkspace,
		PermissionDetailKey: "path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path, defaulting to the workspace root",
				},
				"offset": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Visible entry offset; use next_offset from a truncated result",
				},
				"continuation": map[string]any{
					"type":        "string",
					"description": "Snapshot token from a truncated result; send it with next_offset to detect directory changes",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"maximum":     maxDirectoryEntries,
					"description": "Maximum visible entries to return; defaults to 1000",
				},
			},
			"additionalProperties": false,
		},
		InputAliases: map[string][]string{"path": {"dir_path", "directory"}},
	}
}

func (h listDirHandler) PermissionDetail(arguments json.RawMessage) string {
	arguments = tool.NormalizeArguments(h.Definition(), arguments)
	var input listDirInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "."
	}
	targetPath := strings.TrimSpace(input.Path)
	if targetPath == "" {
		return "."
	}
	return targetPath
}

func (h listDirHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("list_dir workspace is required")
	}
	call.Arguments = tool.NormalizeArguments(h.Definition(), call.Arguments)
	var input listDirInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode list_dir arguments", err)
	}

	targetPath := strings.TrimSpace(input.Path)
	if targetPath == "" {
		targetPath = "."
	}
	if input.Offset < 0 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "list_dir offset must be non-negative")
	}
	if input.Limit < 0 || input.Limit > maxDirectoryEntries {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "list_dir limit must be between 1 and 1000")
	}
	if input.Limit == 0 {
		input.Limit = maxDirectoryEntries
	}

	resolvedPath, err := h.workspace.ResolveRead(ctx, targetPath)
	if err != nil {
		return tool.Result{}, err
	}
	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return tool.Result{}, fmt.Errorf("list %q: %w", targetPath, err)
	}

	continuation, err := continuationToken("list_dir", struct {
		Path string `json:"path"`
	}{Path: targetPath}, "")
	if err != nil {
		return tool.Result{}, err
	}
	if input.Continuation != "" && input.Continuation != continuation {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeStaleContinuation, "list_dir continuation is stale; restart from offset 0")
	}

	allocHint := len(entries)
	if allocHint > input.Limit {
		allocHint = input.Limit
	}
	var output strings.Builder
	output.Grow(allocHint * 48)

	emitted := 0
	visibleSeen := 0
	truncated := false

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return tool.Result{}, fmt.Errorf("list %q: %w", targetPath, err)
		}
		childPath := filepath.Join(resolvedPath, entry.Name())
		if h.workspace.IsProtected(childPath) {
			continue
		}

		entryType := entry.Type()
		if entryType&os.ModeSymlink != 0 {
			if err := h.workspace.CheckAbsoluteRead(ctx, childPath); err != nil {
				continue
			}
		}
		if visibleSeen < input.Offset {
			visibleSeen++
			continue
		}
		if emitted >= input.Limit {
			truncated = true
			break
		}
		visibleSeen++

		// Symlink handling with destination and directory indicator
		if entryType&os.ModeSymlink != 0 {
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
	}

	var nextOffset *int64
	if truncated {
		next := int64(input.Offset + emitted)
		nextOffset = &next
		output.WriteString(fmt.Sprintf("[directory output truncated; continue with offset=%d]\n", next))
	}

	return tool.Result{
		CallID:     call.ID,
		ToolName:   call.Name,
		Output:     output.String(),
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
