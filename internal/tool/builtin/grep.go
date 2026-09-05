package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/projectTHORN/proton/internal/glob"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/workspace"
)

const (
	maxGrepResults     = 100
	maxGrepOutputBytes = 1 * 1024 * 1024
	maxGrepLineLength  = 2000
)

var grepBufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 64*1024)
		return &b
	},
}

var errGrepLimit = errors.New("grep result limit reached")

type grepHandler struct {
	workspace *workspace.Workspace
}

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
	Include string `json:"include"`
	Offset  int    `json:"offset,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type grepPageState struct {
	offset  int
	limit   int
	seen    int
	emitted int
	output  *strings.Builder
}

// NewGrep returns the bounded regular-expression search adapter.
func NewGrep(workspaceRoot *workspace.Workspace) tool.Handler {
	return grepHandler{workspace: workspaceRoot}
}

func (grepHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "grep",
		Description:         "Search workspace files with a regular expression.",
		Kind:                tool.KindGrep,
		Mutability:          tool.MutabilityReadOnly,
		PermissionDetailKey: "path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
				"path":    map[string]any{"type": "string"},
				"include": map[string]any{
					"type":        "string",
					"description": "Optional filename glob, such as *.go",
				},
				"offset": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Match offset to skip; use next_offset from a truncated result",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     maxGrepResults,
					"description": "Maximum matches to return; defaults to 100",
				},
			},
			"required": []string{"pattern"},
		},
	}
}

func (h grepHandler) PermissionDetail(arguments json.RawMessage) string {
	var input grepInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "."
	}
	targetPath := strings.TrimSpace(input.Path)
	if targetPath == "" {
		return "."
	}
	return targetPath
}

func (h grepHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("grep workspace is required")
	}
	var input grepInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode grep arguments: %w", err)
	}
	input.Pattern = strings.TrimSpace(input.Pattern)
	if input.Pattern == "" {
		return tool.Result{}, fmt.Errorf("grep pattern is required")
	}
	matcher, err := regexp.Compile(input.Pattern)
	if err != nil {
		return tool.Result{}, fmt.Errorf("compile grep pattern: %w", err)
	}
	if input.Include != "" {
		if _, err := filepath.Match(input.Include, "probe"); err != nil {
			return tool.Result{}, fmt.Errorf("invalid grep include glob: %w", err)
		}
	}
	if input.Offset < 0 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "grep offset must be non-negative")
	}
	if input.Limit < 0 || input.Limit > maxGrepResults {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "grep limit must be between 1 and 100")
	}
	if input.Limit == 0 {
		input.Limit = maxGrepResults
	}
	searchPath := input.Path
	if strings.TrimSpace(searchPath) == "" {
		searchPath = "."
	}
	resolvedPath, err := h.workspace.ResolveRead(ctx, searchPath)
	if err != nil {
		return tool.Result{}, err
	}

	var output strings.Builder
	page := grepPageState{
		offset: input.Offset,
		limit:  input.Limit,
		output: &output,
	}
	truncated := false
	walkErr := filepath.WalkDir(resolvedPath, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path != resolvedPath && h.workspace.IsProtected(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if input.Include != "" {
			relSearch, _ := filepath.Rel(resolvedPath, path)
			relWork, _ := h.workspace.RelRead(path)
			if !matchGrepInclude(input.Include, entry.Name(), relSearch, relWork) {
				return nil
			}
		}
		scanErr := scanGrepFile(ctx, h.workspace, path, matcher, &page)
		if scanErr != nil {
			if errors.Is(scanErr, errGrepLimit) {
				truncated = true
				return errGrepLimit
			}
			return scanErr
		}
		return nil
	})
	if errors.Is(walkErr, errGrepLimit) {
		walkErr = nil
	}
	if walkErr != nil {
		return tool.Result{}, fmt.Errorf("grep workspace: %w", walkErr)
	}

	var nextOffset *int64
	if truncated {
		next := int64(input.Offset + page.emitted)
		nextOffset = &next
		output.WriteString(fmt.Sprintf("[grep output truncated; continue with offset=%d]\n", next))
	}
	return tool.Result{
		CallID:     call.ID,
		ToolName:   call.Name,
		Output:     output.String(),
		Truncated:  truncated,
		NextOffset: nextOffset,
	}, nil
}

func scanGrepFile(
	ctx context.Context,
	workspaceRoot *workspace.Workspace,
	path string,
	matcher *regexp.Regexp,
	page *grepPageState,
) (returnErr error) {
	if err := workspaceRoot.CheckAbsoluteRead(ctx, path); err != nil {
		if errors.Is(err, workspace.ErrProtectedPath) || errors.Is(err, workspace.ErrOutsideWorkspace) {
			return nil
		}
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && returnErr == nil {
			returnErr = fmt.Errorf("close %q: %w", path, closeErr)
		}
	}()

	relative, err := workspaceRoot.RelRead(path)
	if err != nil {
		return fmt.Errorf("relative grep path: %w", err)
	}
	relative = filepath.ToSlash(relative)
	scanner := bufio.NewScanner(io.LimitReader(file, maxEditFileBytes+1))
	bufPtr := grepBufferPool.Get().(*[]byte)
	defer grepBufferPool.Put(bufPtr)
	scanner.Buffer(*bufPtr, maxEditFileBytes)
	lineNumber := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("grep %q: %w", relative, err)
		}
		lineNumber++
		lineBytes := scanner.Bytes()
		if !matcher.Match(lineBytes) {
			continue
		}
		page.seen++
		if page.seen <= page.offset {
			continue
		}
		if page.emitted >= page.limit {
			return errGrepLimit
		}
		line := truncateGrepLine(string(lineBytes))
		entry := fmt.Sprintf("%s:%d:%s\n", relative, lineNumber, line)
		if page.output.Len()+len(entry) > maxGrepOutputBytes {
			return errGrepLimit
		}
		page.output.WriteString(entry)
		page.emitted++
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return fmt.Errorf("scan %q: %w", relative, scanErr)
	}
	return nil
}

func truncateGrepLine(line string) string {
	if len(line) <= maxGrepLineLength {
		return line
	}
	if utf8.RuneCountInString(line) <= maxGrepLineLength {
		return line
	}
	count := 0
	for i := range line {
		if count == maxGrepLineLength {
			return line[:i] + "…"
		}
		count++
	}
	return line + "…"
}

func matchGrepInclude(pattern string, name string, relPaths ...string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "/")
	if glob.Match(pattern, name) {
		return true
	}
	for _, rel := range relPaths {
		if rel == "" || rel == "." {
			continue
		}
		rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
		if glob.Match(pattern, rel) {
			return true
		}
		if strings.HasPrefix(pattern, "**/") && glob.Match(strings.TrimPrefix(pattern, "**/"), rel) {
			return true
		}
	}
	return false
}
