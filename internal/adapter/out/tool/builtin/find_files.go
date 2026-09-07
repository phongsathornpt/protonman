package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/projectTHORN/proton/internal/base/glob"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/core/workspace"
)

const maxFindFilesResults = 1000

type findFilesHandler struct {
	workspace *workspace.Workspace
}

type findFilesInput struct {
	Pattern      string `json:"pattern,omitempty"`
	Path         string `json:"path,omitempty"`
	Type         string `json:"type,omitempty"`
	MaxDepth     int    `json:"max_depth,omitempty"`
	Offset       int    `json:"offset,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
}

func NewFindFiles(workspaceRoot *workspace.Workspace) tool.Handler {
	return findFilesHandler{workspace: workspaceRoot}
}

func (findFilesHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "find_files",
		Description:         "Find workspace paths recursively by glob pattern. Prefer this over shell find for repository file discovery.",
		Kind:                tool.KindForName("find_files"),
		Mutability:          tool.MutabilityReadOnly,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyWorkspaceRead},
		Evidence:            tool.EvidenceWorkspace,
		PermissionDetailKey: "path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern":      map[string]any{"type": "string", "description": "Glob matched against workspace-relative path and basename; defaults to *"},
				"path":         map[string]any{"type": "string", "description": "Directory to search recursively; defaults to workspace root"},
				"type":         map[string]any{"type": "string", "enum": []string{"any", "file", "dir"}, "description": "Path type filter; defaults to file"},
				"max_depth":    map[string]any{"type": "integer", "minimum": 0, "description": "Maximum depth below path; 0 means unlimited"},
				"offset":       map[string]any{"type": "integer", "minimum": 0, "description": "Match offset to skip; use next_offset from a truncated result"},
				"continuation": map[string]any{"type": "string", "description": "Snapshot token from a truncated result; send it with next_offset to detect tree changes"},
				"limit":        map[string]any{"type": "integer", "minimum": 0, "maximum": maxFindFilesResults, "description": "Maximum paths to return; defaults to 1000"},
			},
			"additionalProperties": false,
		},
	}
}

func (h findFilesHandler) PermissionDetail(arguments json.RawMessage) string {
	var input findFilesInput
	if json.Unmarshal(arguments, &input) != nil || strings.TrimSpace(input.Path) == "" {
		return "."
	}
	return strings.TrimSpace(input.Path)
}

func (h findFilesHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("find_files workspace is required")
	}
	var input findFilesInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode find_files arguments", err)
	}
	if err := normalizeFindFilesInput(&input); err != nil {
		return tool.Result{}, err
	}
	resolvedRoot, err := h.workspace.ResolveRead(ctx, input.Path)
	if err != nil {
		return tool.Result{}, err
	}
	info, err := os.Stat(resolvedRoot)
	if err != nil {
		return tool.Result{}, fmt.Errorf("stat find_files root %q: %w", input.Path, err)
	}
	if !info.IsDir() {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q is not a directory; use read_file instead", input.Path))
	}

	query := struct {
		Pattern  string `json:"pattern"`
		Path     string `json:"path"`
		Type     string `json:"type"`
		MaxDepth int    `json:"max_depth"`
	}{input.Pattern, input.Path, input.Type, input.MaxDepth}

	token, err := continuationToken("find_files", query, "")
	if err != nil {
		return tool.Result{}, err
	}
	if input.Continuation != "" && input.Continuation != token {
		return tool.Result{}, stalePaginationError("find_files", "find_files continuation does not match this query; restart from offset 0", call.Arguments)
	}

	var output strings.Builder
	seen, emitted := 0, 0
	truncated := false
	walkErr := filepath.WalkDir(resolvedRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == resolvedRoot {
			return nil
		}
		if h.workspace.IsProtected(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() && isIgnoredSearchDir(entry.Name()) {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if err := h.workspace.CheckAbsoluteRead(ctx, path); err != nil {
				return nil
			}
		}
		relSearch, err := filepath.Rel(resolvedRoot, path)
		if err != nil {
			return err
		}
		depth := pathDepth(relSearch)
		if input.MaxDepth > 0 && depth > input.MaxDepth {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !findFilesTypeMatches(input.Type, entry) {
			return nil
		}
		relSearch = filepath.ToSlash(relSearch)
		if !glob.Match(input.Pattern, relSearch) && !glob.Match(input.Pattern, entry.Name()) {
			return nil
		}
		display, err := h.workspace.RelRead(path)
		if err != nil {
			return err
		}
		display = filepath.ToSlash(display)
		if seen < input.Offset {
			seen++
			return nil
		}
		if emitted >= input.Limit {
			truncated = true
			return filepath.SkipAll
		}
		seen++
		output.WriteString(findFilesEntryType(entry))
		output.WriteByte(' ')
		output.WriteString(display)
		if entry.IsDir() {
			output.WriteByte('/')
		}
		output.WriteByte('\n')
		emitted++
		return nil
	})
	if walkErr != nil {
		return tool.Result{}, fmt.Errorf("find files under %q: %w", input.Path, walkErr)
	}

	var nextOffset *int64
	if truncated {
		next := int64(input.Offset + emitted)
		nextOffset = &next
		output.WriteString(fmt.Sprintf("[output truncated; continue with offset=%d]", next))
	}
	return tool.Result{
		CallID:     call.ID,
		ToolName:   call.Name,
		Output:     strings.TrimSuffix(output.String(), "\n"),
		Truncated:  truncated,
		NextOffset: nextOffset,
		Continuation: func() string {
			if truncated {
				return token
			}
			return ""
		}(),
		Pagination: paginationState(truncated, "offset", nextOffset, nil, token),
	}, nil
}

func normalizeFindFilesInput(input *findFilesInput) error {
	input.Pattern = filepath.ToSlash(strings.TrimSpace(input.Pattern))
	if input.Pattern == "" {
		input.Pattern = "*"
	}
	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		input.Path = "."
	}
	input.Type = strings.ToLower(strings.TrimSpace(input.Type))
	if input.Type == "" {
		input.Type = "file"
	}
	if input.Type != "any" && input.Type != "file" && input.Type != "dir" {
		return tool.NewToolError(tool.ErrorCodeInvalidArguments, "find_files type must be any, file, or dir")
	}
	if input.MaxDepth < 0 || input.Offset < 0 || input.Limit < 0 || input.Limit > maxFindFilesResults {
		return tool.NewToolError(tool.ErrorCodeInvalidArguments, "find_files max_depth/offset must be non-negative and limit must be between 1 and 1000")
	}
	if input.Limit == 0 {
		input.Limit = maxFindFilesResults
	}
	return nil
}

func pathDepth(path string) int {
	clean := filepath.Clean(path)
	if clean == "." || clean == "" {
		return 0
	}
	return strings.Count(filepath.ToSlash(clean), "/") + 1
}

func findFilesTypeMatches(kind string, entry os.DirEntry) bool {
	switch kind {
	case "file":
		return !entry.IsDir() && entry.Type()&os.ModeSymlink == 0
	case "dir":
		return entry.IsDir()
	default:
		return true
	}
}

func findFilesEntryType(entry os.DirEntry) string {
	if entry.Type()&os.ModeSymlink != 0 {
		return "link"
	}
	if entry.IsDir() {
		return "dir"
	}
	return "file"
}
