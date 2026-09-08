package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/phongsathornpt/proton/internal/base/glob"
	"github.com/phongsathornpt/proton/internal/core/tool"
	"github.com/phongsathornpt/proton/internal/core/workspace"
)

const (
	maxInspectMatches     = 100
	maxInspectFiles       = 10000
	maxInspectOutputBytes = 1 * 1024 * 1024
	maxInspectFileBytes   = 4 * 1024 * 1024
	maxInspectContext     = 100
)

type inspectCodeHandler struct {
	workspace *workspace.Workspace
}

type inspectCodeContext struct {
	Before int `json:"before,omitempty"`
	After  int `json:"after,omitempty"`
}

type inspectCodeInput struct {
	Path       string             `json:"path,omitempty"`
	Include    []string           `json:"include,omitempty"`
	Exclude    []string           `json:"exclude,omitempty"`
	Query      string             `json:"query"`
	Mode       string             `json:"mode,omitempty"`
	Context    inspectCodeContext `json:"context,omitempty"`
	MaxFiles   int                `json:"max_files,omitempty"`
	MaxMatches int                `json:"max_matches,omitempty"`
}

type inspectCodeMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Column  int    `json:"column,omitempty"`
	Excerpt string `json:"excerpt"`
}

type inspectCodeStats struct {
	FilesScanned int `json:"files_scanned"`
	FilesMatched int `json:"files_matched"`
	Matches      int `json:"matches"`
}

type inspectCodeOutput struct {
	Matches   []inspectCodeMatch `json:"matches"`
	Stats     inspectCodeStats   `json:"stats"`
	Truncated bool               `json:"truncated"`
}

func NewInspectCode(workspaceRoot *workspace.Workspace) tool.Handler {
	return inspectCodeHandler{workspace: workspaceRoot}
}

func (inspectCodeHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "inspect_code",
		Description:         "Inspect repository source code in one call. Use for multi-file source inspection that would otherwise require find_files + grep + multiple read_file calls. Supports recursive file filtering, literal/regex matching, and bounded context extraction. Prefer this over bash, Python, Node, find, grep, rg, cat, awk, or custom scripts for searching or inspecting workspace source files.",
		Kind:                tool.KindForName("inspect_code"),
		Mutability:          tool.MutabilityReadOnly,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyWorkspaceRead},
		Evidence:            tool.EvidenceWorkspace,
		PermissionDetailKey: "path",
		InputSchema:         inspectCodeInputSchema(),
		OutputSchema:        inspectCodeOutputSchema(),
	}
}

func inspectCodeInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "Directory to inspect recursively; defaults to workspace root"},
			"include": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional include globs such as **/*.go"},
			"exclude": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional exclude globs such as **/*_test.go"},
			"query":   map[string]any{"type": "string", "description": "Literal text or regular expression to match"},
			"mode":    map[string]any{"type": "string", "enum": []string{"literal", "regex"}, "description": "Match mode; defaults to literal"},
			"context": map[string]any{"type": "object", "properties": map[string]any{
				"before": map[string]any{"type": "integer", "minimum": 0, "maximum": maxInspectContext},
				"after":  map[string]any{"type": "integer", "minimum": 0, "maximum": maxInspectContext},
			}, "additionalProperties": false},
			"max_files":   map[string]any{"type": "integer", "minimum": 0, "maximum": maxInspectFiles, "description": "Maximum files to scan; defaults to 10000"},
			"max_matches": map[string]any{"type": "integer", "minimum": 0, "maximum": maxInspectMatches, "description": "Maximum matches to return; defaults to 100"},
		},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
}

func inspectCodeOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"matches": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"line":    map[string]any{"type": "integer"},
					"column":  map[string]any{"type": "integer"},
					"excerpt": map[string]any{"type": "string"},
				}, "required": []string{"path", "line", "excerpt"}, "additionalProperties": false,
			}},
			"stats": map[string]any{"type": "object", "properties": map[string]any{
				"files_scanned": map[string]any{"type": "integer"},
				"files_matched": map[string]any{"type": "integer"},
				"matches":       map[string]any{"type": "integer"},
			}, "required": []string{"files_scanned", "files_matched", "matches"}, "additionalProperties": false},
			"truncated": map[string]any{"type": "boolean"},
		},
		"required":             []string{"matches", "stats", "truncated"},
		"additionalProperties": false,
	}
}

func (h inspectCodeHandler) PermissionDetail(arguments json.RawMessage) string {
	var input inspectCodeInput
	if json.Unmarshal(arguments, &input) != nil || strings.TrimSpace(input.Path) == "" {
		return "."
	}
	return strings.TrimSpace(input.Path)
}

func (h inspectCodeHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.workspace == nil {
		return tool.Result{}, fmt.Errorf("inspect_code workspace is required")
	}
	var input inspectCodeInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode inspect_code arguments", err)
	}
	matcher, err := normalizeInspectCodeInput(&input)
	if err != nil {
		return tool.Result{}, err
	}
	root, err := h.workspace.ResolveRead(ctx, input.Path)
	if err != nil {
		return tool.Result{}, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return tool.Result{}, fmt.Errorf("stat inspect_code root %q: %w", input.Path, err)
	}
	if !info.IsDir() {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("%q is not a directory; use read_file instead", input.Path))
	}

	out := inspectCodeOutput{Matches: make([]inspectCodeMatch, 0)}
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
			if isIgnoredSearchDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || isIgnoredGrepExtension(entry.Name()) {
			return nil
		}
		rel, err := h.workspace.RelRead(path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !inspectPathMatches(rel, entry.Name(), input.Include, input.Exclude) {
			return nil
		}
		if out.Stats.FilesScanned >= input.MaxFiles {
			out.Truncated = true
			return filepath.SkipAll
		}
		out.Stats.FilesScanned++
		matches, err := inspectFile(path, rel, input, matcher, input.MaxMatches-len(out.Matches), maxInspectOutputBytes-outputBytes)
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
			if len(out.Matches) >= input.MaxMatches || outputBytes >= maxInspectOutputBytes {
				out.Truncated = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	if walkErr != nil {
		return tool.Result{}, fmt.Errorf("inspect code under %q: %w", input.Path, walkErr)
	}
	out.Stats.FilesMatched = len(matchedFiles)
	out.Stats.Matches = len(out.Matches)
	structured, err := json.Marshal(out)
	if err != nil {
		return tool.Result{}, fmt.Errorf("encode inspect_code result: %w", err)
	}
	return tool.Result{
		CallID: call.ID, ToolName: call.Name,
		Output: formatInspectCodeOutput(out), StructuredOutput: structured, Truncated: out.Truncated,
	}, nil
}

type inspectMatcher struct {
	literal []byte
	regex   *regexp.Regexp
}

func normalizeInspectCodeInput(input *inspectCodeInput) (inspectMatcher, error) {
	input.Path = strings.TrimSpace(input.Path)
	if input.Path == "" {
		input.Path = "."
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" {
		return inspectMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "inspect_code query is required")
	}
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	if input.Mode == "" {
		input.Mode = "literal"
	}
	if input.Mode != "literal" && input.Mode != "regex" {
		return inspectMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "inspect_code mode must be literal or regex")
	}
	if input.Context.Before < 0 || input.Context.Before > maxInspectContext || input.Context.After < 0 || input.Context.After > maxInspectContext {
		return inspectMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "inspect_code context must be between 0 and 100 lines")
	}
	if input.MaxFiles < 0 || input.MaxFiles > maxInspectFiles || input.MaxMatches < 0 || input.MaxMatches > maxInspectMatches {
		return inspectMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "inspect_code limits are out of range")
	}
	if input.MaxFiles == 0 {
		input.MaxFiles = maxInspectFiles
	}
	if input.MaxMatches == 0 {
		input.MaxMatches = maxInspectMatches
	}
	for _, pattern := range append(append([]string{}, input.Include...), input.Exclude...) {
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		if _, err := filepath.Match(pattern, "probe"); err != nil {
			return inspectMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "inspect_code contains an invalid glob: "+pattern)
		}
	}
	if input.Mode == "literal" {
		return inspectMatcher{literal: []byte(input.Query)}, nil
	}
	re, err := regexp.Compile(input.Query)
	if err != nil {
		return inspectMatcher{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "invalid inspect_code regex: "+err.Error())
	}
	return inspectMatcher{regex: re}, nil
}

func inspectPathMatches(rel, name string, includes, excludes []string) bool {
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

func inspectFile(path, rel string, input inspectCodeInput, matcher inspectMatcher, remainingMatches, remainingBytes int) ([]inspectCodeMatch, error) {
	if remainingMatches <= 0 || remainingBytes <= 0 {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", rel, err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxInspectFileBytes {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", rel, err)
	}
	if isBinaryData(data[:min(len(data), 1024)]) {
		return nil, nil
	}
	lines := bytes.Split(data, []byte("\n"))
	result := make([]inspectCodeMatch, 0)
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
		result = append(result, inspectCodeMatch{Path: rel, Line: i + 1, Column: column + 1, Excerpt: excerpt})
		usedBytes += entryBytes
		if len(result) >= remainingMatches {
			break
		}
	}
	return result, nil
}

func formatInspectCodeOutput(out inspectCodeOutput) string {
	var b strings.Builder
	for i, match := range out.Matches {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "%s:%d:%d\n%s", match.Path, match.Line, match.Column, match.Excerpt)
	}
	fmt.Fprintf(&b, "\n\n[scanned %d files; matched %d files; %d matches", out.Stats.FilesScanned, out.Stats.FilesMatched, out.Stats.Matches)
	if out.Truncated {
		b.WriteString("; output truncated")
	}
	b.WriteByte(']')
	return strings.TrimSpace(b.String())
}
