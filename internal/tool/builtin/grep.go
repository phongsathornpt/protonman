package builtin

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	Pattern      string `json:"pattern"`
	Path         string `json:"path"`
	Include      string `json:"include"`
	Offset       int    `json:"offset,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
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
				"continuation": map[string]any{
					"type":        "string",
					"description": "Snapshot token from a truncated result; send it with next_offset to detect workspace changes",
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

	grepMatcher, err := newGrepMatcher(input.Pattern)
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
	scanEnabled := true
	snapshotHash := sha256.New()
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
			if entry.Name() == ".git" && path != resolvedPath {
				return filepath.SkipDir
			}
			return hashGrepSnapshotEntry(snapshotHash, resolvedPath, path, entry)
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
		if err := hashGrepSnapshotEntry(snapshotHash, resolvedPath, path, entry); err != nil {
			return err
		}
		if !scanEnabled {
			return nil
		}
		scanErr := scanGrepFile(ctx, h.workspace, path, grepMatcher, &page)
		if scanErr != nil {
			if errors.Is(scanErr, errGrepLimit) {
				truncated = true
				scanEnabled = false
				return nil
			}
			return scanErr
		}
		return nil
	})
	if walkErr != nil {
		return tool.Result{}, fmt.Errorf("grep workspace: %w", walkErr)
	}
	snapshot := hex.EncodeToString(snapshotHash.Sum(nil))
	continuation, err := continuationToken("grep", struct{ Pattern, Path, Include string }{input.Pattern, searchPath, input.Include}, snapshot)
	if err != nil {
		return tool.Result{}, err
	}
	if input.Continuation != "" && input.Continuation != continuation {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeStaleContinuation, "grep continuation is stale; restart from offset 0")
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
		Continuation: func() string {
			if truncated {
				return continuation
			}
			return ""
		}(),
	}, nil
}

type grepMatcher struct {
	regexp   *regexp.Regexp
	literal  []byte
	prefix   []byte
	complete bool
}

func newGrepMatcher(pattern string) (grepMatcher, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return grepMatcher{}, fmt.Errorf("compile grep pattern: %w", err)
	}
	prefix, complete := re.LiteralPrefix()
	m := grepMatcher{regexp: re, complete: complete}
	if complete && prefix != "" {
		m.literal = []byte(prefix)
	} else if len(prefix) >= 3 {
		m.prefix = []byte(prefix)
	}
	return m, nil
}

func (m grepMatcher) Match(line []byte) bool {
	if m.complete && len(m.literal) > 0 {
		return bytes.Contains(line, m.literal)
	}
	if len(m.prefix) > 0 && !bytes.Contains(line, m.prefix) {
		return false
	}
	return m.regexp.Match(line)
}

func hashGrepSnapshotEntry(h hash.Hash, root, path string, entry os.DirEntry) error {
	info, err := entry.Info()
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	_, _ = h.Write([]byte(filepath.ToSlash(rel)))
	var encoded [24]byte
	binary.LittleEndian.PutUint64(encoded[0:8], uint64(info.Size()))
	binary.LittleEndian.PutUint64(encoded[8:16], uint64(info.ModTime().UnixNano()))
	binary.LittleEndian.PutUint64(encoded[16:24], uint64(info.Mode()))
	_, _ = h.Write(encoded[:])
	return nil
}

func scanGrepFile(
	ctx context.Context,
	workspaceRoot *workspace.Workspace,
	path string,
	matcher grepMatcher,
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
		cut, shortened := grepLineCut(lineBytes)
		var lineNumberBuf [20]byte
		lineNumberText := strconv.AppendInt(lineNumberBuf[:0], int64(lineNumber), 10)
		entryBytes := len(relative) + 1 + len(lineNumberText) + 1 + cut + 1
		if shortened {
			entryBytes += len("…")
		}
		if page.output.Len()+entryBytes > maxGrepOutputBytes {
			return errGrepLimit
		}
		page.output.WriteString(relative)
		page.output.WriteByte(':')
		_, _ = page.output.Write(lineNumberText)
		page.output.WriteByte(':')
		_, _ = page.output.Write(lineBytes[:cut])
		if shortened {
			page.output.WriteString("…")
		}
		page.output.WriteByte('\n')
		page.emitted++
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return fmt.Errorf("scan %q: %w", relative, scanErr)
	}
	return nil
}

func grepLineCut(line []byte) (int, bool) {
	if len(line) <= maxGrepLineLength {
		return len(line), false
	}
	position := 0
	for count := 0; position < len(line); count++ {
		if count == maxGrepLineLength {
			return position, true
		}
		_, size := utf8.DecodeRune(line[position:])
		position += size
	}
	return len(line), false
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
