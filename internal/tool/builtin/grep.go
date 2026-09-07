package builtin

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	offset   int
	limit    int
	seen     int
	emitted  int
	output   *strings.Builder
	lastFile string
	lastLine int
}

type grepContinuation struct {
	Version  int    `json:"v"`
	Query    string `json:"q"`
	Snapshot string `json:"s"`
	File     string `json:"f"`
	Line     int    `json:"l"`
	Matches  int    `json:"m"`
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
		Evidence:            tool.EvidenceWorkspace,
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
					"minimum":     0,
					"maximum":     maxGrepResults,
					"description": "Maximum matches to return; defaults to 100",
				},
			},
			"required":             []string{"pattern"},
			"additionalProperties": false,
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
	searchOutputBase, err := h.workspace.RelRead(resolvedPath)
	if err != nil {
		return tool.Result{}, fmt.Errorf("relative grep root: %w", err)
	}
	searchOutputBase = filepath.ToSlash(searchOutputBase)

	grepMatcher, err := newGrepMatcher(input.Pattern)
	if err != nil {
		return tool.Result{}, err
	}
	query := struct{ Pattern, Path, Include string }{input.Pattern, searchPath, input.Include}
	queryHash, err := continuationToken("grep-query", query, "")
	if err != nil {
		return tool.Result{}, err
	}
	var resume grepContinuation
	resumeActive := false
	legacyContinuation := ""
	if input.Continuation != "" {
		decoded, isCursor, decodeErr := decodeGrepContinuation(input.Continuation)
		if decodeErr != nil {
			return tool.Result{}, decodeErr
		}
		if isCursor {
			if decoded.Query != queryHash || decoded.Matches != input.Offset {
				return tool.Result{}, tool.NewToolError(tool.ErrorCodeStaleContinuation, "grep continuation does not match this query or offset; restart from offset 0")
			}
			resume = decoded
			resumeActive = true
		} else {
			legacyContinuation = input.Continuation
		}
	}

	var output strings.Builder
	pageOffset := input.Offset
	if resumeActive {
		pageOffset = 0
	}
	page := grepPageState{
		offset: pageOffset,
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
		relSearch, err := filepath.Rel(resolvedPath, path)
		if err != nil {
			return err
		}
		relSearch = filepath.ToSlash(relSearch)
		if entry.IsDir() {
			if entry.Name() == ".git" && path != resolvedPath {
				return filepath.SkipDir
			}
			if path != resolvedPath {
				if err := h.workspace.CheckAbsoluteRead(ctx, path); err != nil {
					return err
				}
			}
			_, err := hashGrepSnapshotEntry(snapshotHash, relSearch, entry)
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		relWork := joinGrepRelative(searchOutputBase, relSearch)
		if input.Include != "" {
			if !matchGrepInclude(input.Include, entry.Name(), relSearch, relWork) {
				return nil
			}
		}
		info, err := hashGrepSnapshotEntry(snapshotHash, relSearch, entry)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil
		}
		if !scanEnabled {
			return nil
		}
		startLine := 0
		if resumeActive {
			switch strings.Compare(relSearch, resume.File) {
			case -1:
				return nil
			case 0:
				startLine = resume.Line
			}
		}
		scanErr := scanGrepFile(ctx, path, relWork, relSearch, startLine, grepMatcher, &page)
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
	if resumeActive && resume.Snapshot != snapshot {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeStaleContinuation, "grep continuation is stale; restart from offset 0")
	}
	if legacyContinuation != "" {
		legacyToken, tokenErr := continuationToken("grep", query, snapshot)
		if tokenErr != nil {
			return tool.Result{}, tokenErr
		}
		if legacyContinuation != legacyToken {
			return tool.Result{}, tool.NewToolError(tool.ErrorCodeStaleContinuation, "grep continuation is stale; restart from offset 0")
		}
	}

	var nextOffset *int64
	continuation := ""
	if truncated {
		next := int64(input.Offset + page.emitted)
		nextOffset = &next
		continuation, err = encodeGrepContinuation(grepContinuation{
			Version:  1,
			Query:    queryHash,
			Snapshot: snapshot,
			File:     page.lastFile,
			Line:     page.lastLine,
			Matches:  int(next),
		})
		if err != nil {
			return tool.Result{}, err
		}
		output.WriteString(fmt.Sprintf("[grep output truncated; continue with offset=%d]\n", next))
	}
	return tool.Result{
		CallID:       call.ID,
		ToolName:     call.Name,
		Output:       output.String(),
		Truncated:    truncated,
		NextOffset:   nextOffset,
		Continuation: continuation,
	}, nil
}

func encodeGrepContinuation(value grepContinuation) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode grep continuation: %w", err)
	}
	return "g1." + base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeGrepContinuation(value string) (grepContinuation, bool, error) {
	if !strings.HasPrefix(value, "g1.") {
		return grepContinuation{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "g1."))
	if err != nil {
		return grepContinuation{}, true, tool.NewToolError(tool.ErrorCodeStaleContinuation, "grep continuation is malformed; restart from offset 0")
	}
	var decoded grepContinuation
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Version != 1 || decoded.Query == "" || decoded.Snapshot == "" || decoded.File == "" || decoded.Line <= 0 || decoded.Matches <= 0 {
		return grepContinuation{}, true, tool.NewToolError(tool.ErrorCodeStaleContinuation, "grep continuation is malformed; restart from offset 0")
	}
	return decoded, true, nil
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

func hashGrepSnapshotEntry(h hash.Hash, relative string, entry os.DirEntry) (os.FileInfo, error) {
	info, err := entry.Info()
	if err != nil {
		return nil, err
	}
	_, _ = h.Write([]byte(relative))
	var encoded [24]byte
	binary.LittleEndian.PutUint64(encoded[0:8], uint64(info.Size()))
	binary.LittleEndian.PutUint64(encoded[8:16], uint64(info.ModTime().UnixNano()))
	binary.LittleEndian.PutUint64(encoded[16:24], uint64(info.Mode()))
	_, _ = h.Write(encoded[:])
	return info, nil
}

func scanGrepFile(
	ctx context.Context,
	path string,
	relative string,
	cursorFile string,
	startLine int,
	matcher grepMatcher,
	page *grepPageState,
) (returnErr error) {
	// path is a non-symlink candidate produced by WalkDir from a root already
	// validated by ResolveRead. The walk filters protected paths and never
	// descends through symlink entries, so repeating CheckAbsoluteRead here
	// would re-run EvalSymlinks for every file without strengthening the boundary.
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && returnErr == nil {
			returnErr = fmt.Errorf("close %q: %w", path, closeErr)
		}
	}()

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
		if lineNumber <= startLine {
			continue
		}
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
		page.lastFile = cursorFile
		page.lastLine = lineNumber
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

func joinGrepRelative(base, relative string) string {
	if relative == "" || relative == "." {
		return base
	}
	if base == "" || base == "." {
		return relative
	}
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(relative, "/")
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
