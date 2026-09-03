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

	"github.com/projectTHORN/proton/internal/workspace"
	"github.com/projectTHORN/proton/internal/tool"
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
			},
			"required": []string{"pattern"},
		},
	}
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
	searchPath := input.Path
	if strings.TrimSpace(searchPath) == "" {
		searchPath = "."
	}
	resolvedPath, err := h.workspace.Resolve(ctx, searchPath)
	if err != nil {
		return tool.Result{}, err
	}

	var output strings.Builder
	matchCount := 0
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
			matched, matchErr := filepath.Match(input.Include, entry.Name())
			if matchErr != nil {
				return fmt.Errorf("match grep include glob: %w", matchErr)
			}
			if !matched {
				return nil
			}
		}
		fileMatches, scanErr := scanGrepFile(ctx, h.workspace, path, matcher, &output, &matchCount)
		if scanErr != nil {
			if errors.Is(scanErr, errGrepLimit) {
				truncated = true
				return errGrepLimit
			}
			return scanErr
		}
		if fileMatches && (matchCount >= maxGrepResults || output.Len() >= maxGrepOutputBytes) {
			truncated = true
			return errGrepLimit
		}
		return nil
	})
	if errors.Is(walkErr, errGrepLimit) {
		walkErr = nil
	}
	if walkErr != nil {
		return tool.Result{}, fmt.Errorf("grep workspace: %w", walkErr)
	}
	if truncated {
		output.WriteString("[grep output truncated at configured limit]\n")
	}
	return tool.Result{
		CallID:    call.ID,
		ToolName:  call.Name,
		Output:    output.String(),
		Truncated: truncated,
	}, nil
}

func scanGrepFile(
	ctx context.Context,
	workspaceRoot *workspace.Workspace,
	path string,
	matcher *regexp.Regexp,
	output *strings.Builder,
	matchCount *int,
) (matchedFile bool, returnErr error) {
	if err := workspaceRoot.CheckAbsolute(ctx, path); err != nil {
		if errors.Is(err, workspace.ErrProtectedPath) || errors.Is(err, workspace.ErrOutsideWorkspace) {
			return false, nil
		}
		return false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && returnErr == nil {
			returnErr = fmt.Errorf("close %q: %w", path, closeErr)
		}
	}()

	relative, err := filepath.Rel(workspaceRoot.Root(), path)
	if err != nil {
		return false, fmt.Errorf("relative grep path: %w", err)
	}
	relative = filepath.ToSlash(relative)
	scanner := bufio.NewScanner(io.LimitReader(file, maxEditFileBytes+1))
	bufPtr := grepBufferPool.Get().(*[]byte)
	defer grepBufferPool.Put(bufPtr)
	scanner.Buffer(*bufPtr, maxEditFileBytes)
	lineNumber := 0
	matchedFile = false
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return false, fmt.Errorf("grep %q: %w", relative, err)
		}
		lineNumber++
		lineBytes := scanner.Bytes()
		if !matcher.Match(lineBytes) {
			continue
		}
		matchedFile = true
		*matchCount = *matchCount + 1
		line := truncateGrepLine(string(lineBytes))
		entry := fmt.Sprintf("%s:%d:%s\n", relative, lineNumber, line)
		if output.Len()+len(entry) > maxGrepOutputBytes {
			return true, errGrepLimit
		}
		output.WriteString(entry)
		if *matchCount >= maxGrepResults {
			return true, errGrepLimit
		}
	}
	scanErr := scanner.Err()
	if scanErr != nil {
		return false, fmt.Errorf("scan %q: %w", relative, scanErr)
	}
	return matchedFile, nil
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
