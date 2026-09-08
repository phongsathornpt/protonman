package readfile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/phongsathornpt/protonman/internal/base/glob"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

const (
	maxSourceMatches     = 100
	maxSourceFiles       = 10000
	maxSourceOutputBytes = 1 * 1024 * 1024
	maxSourceFileBytes   = 4 * 1024 * 1024
	maxSourceContext     = 100
)

type sourceContext struct {
	Before int `json:"before,omitempty"`
	After  int `json:"after,omitempty"`
}
type sourceMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column,omitempty"`
	Excerpt string `json:"excerpt"`
}
type sourceStats struct {
	FilesScanned int `json:"files_scanned"`
	FilesMatched int `json:"files_matched"`
	Matches      int `json:"matches"`
}
type sourceOutput struct {
	Matches   []sourceMatch `json:"matches"`
	Stats     sourceStats   `json:"stats"`
	Truncated bool          `json:"truncated"`
}
type sourceMatcher struct {
	literal []byte
	regex   *regexp.Regexp
}

func (h readFileHandler) readSource(ctx context.Context, input readFileInput, call tool.Call) (tool.Result, error) {
	matcher, err := normalizeSourceInput(&input)
	if err != nil {
		return tool.Result{}, err
	}
	root, err := h.workspace.ResolveExistingRead(ctx, input.Path)
	if err != nil {
		return tool.Result{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return tool.Result{}, fmt.Errorf("stat read source root %q: %w", input.Path, err)
	}
	if !info.IsDir() {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q is not a directory; use text view instead", input.Path))
	}

	out := sourceOutput{Matches: make([]sourceMatch, 0)}
	matchedFiles := make(map[string]struct{})
	outputBytes := 0
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if h.workspace.IsProtected(path) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if ignoredSourceDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || ignoredSourceExtension(entry.Name()) {
			return nil
		}
		rel, err := h.workspace.RelRead(path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !sourcePathMatches(rel, entry.Name(), input.Include, input.Exclude) {
			return nil
		}
		if out.Stats.FilesScanned >= input.MaxFiles {
			out.Truncated = true
			return filepath.SkipAll
		}
		out.Stats.FilesScanned++
		matches, err := inspectSourceFile(path, rel, input, matcher, input.MaxMatches-len(out.Matches), maxSourceOutputBytes-outputBytes)
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return nil
		}
		matchedFiles[rel] = struct{}{}
		for _, match := range matches {
			out.Matches = append(out.Matches, match)
			outputBytes += len(match.Path) + len(match.Excerpt) + 32
			if len(out.Matches) >= input.MaxMatches || outputBytes >= maxSourceOutputBytes {
				out.Truncated = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	if walkErr != nil {
		return tool.Result{}, fmt.Errorf("inspect source under %q: %w", input.Path, walkErr)
	}
	out.Stats.FilesMatched = len(matchedFiles)
	out.Stats.Matches = len(out.Matches)
	structured, err := json.Marshal(out)
	if err != nil {
		return tool.Result{}, fmt.Errorf("encode read source result: %w", err)
	}
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: formatSourceOutput(out), StructuredOutput: structured, Truncated: out.Truncated}, nil
}

func normalizeSourceInput(input *readFileInput) (sourceMatcher, error) {
	if input.Path == "" {
		input.Path = "."
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" {
		return sourceMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read source query is required")
	}
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	if input.Mode == "" {
		input.Mode = "literal"
	}
	if input.Mode != "literal" && input.Mode != "regex" {
		return sourceMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read source mode must be literal or regex")
	}
	if input.Context.Before < 0 || input.Context.Before > maxSourceContext || input.Context.After < 0 || input.Context.After > maxSourceContext {
		return sourceMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read source context must be between 0 and 100 lines")
	}
	if input.MaxFiles < 0 || input.MaxFiles > maxSourceFiles || input.MaxMatches < 0 || input.MaxMatches > maxSourceMatches {
		return sourceMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read source limits are out of range")
	}
	if input.MaxFiles == 0 {
		input.MaxFiles = maxSourceFiles
	}
	if input.MaxMatches == 0 {
		input.MaxMatches = maxSourceMatches
	}
	for _, pattern := range append(append([]string{}, input.Include...), input.Exclude...) {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		if _, err := filepath.Match(pattern, "probe"); err != nil {
			return sourceMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "read source contains an invalid glob: "+pattern)
		}
	}
	if input.Mode == "literal" {
		return sourceMatcher{literal: []byte(input.Query)}, nil
	}
	re, err := regexp.Compile(input.Query)
	if err != nil {
		return sourceMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "invalid read source regex: "+err.Error())
	}
	return sourceMatcher{regex: re}, nil
}

func sourcePathMatches(rel, name string, includes, excludes []string) bool {
	matched := len(includes) == 0
	for _, pattern := range includes {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern != "" && (glob.Match(pattern, rel) || glob.Match(pattern, name) || strings.HasPrefix(pattern, "**/") && glob.Match(strings.TrimPrefix(pattern, "**/"), rel)) {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}
	for _, pattern := range excludes {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern != "" && (glob.Match(pattern, rel) || glob.Match(pattern, name) || strings.HasPrefix(pattern, "**/") && glob.Match(strings.TrimPrefix(pattern, "**/"), rel)) {
			return false
		}
	}
	return true
}

func inspectSourceFile(path, rel string, input readFileInput, matcher sourceMatcher, remainingMatches, remainingBytes int) ([]sourceMatch, error) {
	if remainingMatches <= 0 || remainingBytes <= 0 {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", rel, err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxSourceFileBytes {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", rel, err)
	}
	if bytes.IndexByte(data[:min(len(data), 1024)], 0) != -1 {
		return nil, nil
	}
	lines := bytes.Split(data, []byte("\n"))
	result := make([]sourceMatch, 0)
	usedBytes := 0
	for i, line := range lines {
		column := -1
		if len(matcher.literal) > 0 {
			column = bytes.Index(line, matcher.literal)
		} else if loc := matcher.regex.FindIndex(line); loc != nil {
			column = loc[0]
		}
		if column < 0 {
			continue
		}
		start := max(0, i-input.Context.Before)
		end := min(len(lines), i+input.Context.After+1)
		excerpt := string(bytes.Join(lines[start:end], []byte("\n")))
		entryBytes := len(rel) + len(excerpt) + 32
		if usedBytes+entryBytes > remainingBytes {
			break
		}
		result = append(result, sourceMatch{Path: rel, Line: i + 1, Column: column + 1, Excerpt: excerpt})
		usedBytes += entryBytes
		if len(result) >= remainingMatches {
			break
		}
	}
	return result, nil
}

func ignoredSourceDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "bin", "obj", "dist", "build", "target", "node_modules", ".cache", ".idea", ".vscode":
		return true
	}
	return false
}
func ignoredSourceExtension(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".exe", ".bin", ".o", ".a", ".so", ".dylib", ".wasm", ".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".pdf", ".mp4", ".mov", ".avi", ".mp3", ".wav", ".ttf", ".otf", ".woff", ".woff2":
		return true
	}
	return false
}
func formatSourceOutput(out sourceOutput) string {
	var b strings.Builder
	for i, m := range out.Matches {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%s:%d:%d\n%s", m.Path, m.Line, m.Column, m.Excerpt)
	}
	fmt.Fprintf(&b, "\n\n[scanned %d files; matched %d files; %d matches", out.Stats.FilesScanned, out.Stats.FilesMatched, out.Stats.Matches)
	if out.Truncated {
		b.WriteString("; output truncated")
	}
	b.WriteByte(']')
	return strings.TrimSpace(b.String())
}
