package readfile

import "github.com/phongsathornpt/protonman/internal/core/tool"

const MaxReadFileBytes = 2 * 1024 * 1024

type readFileInput struct {
	Path         string        `json:"path"`
	View         string        `json:"view,omitempty"`
	Query        string        `json:"query,omitempty"`
	Mode         string        `json:"mode,omitempty"`
	Include      []string      `json:"include,omitempty"`
	Exclude      []string      `json:"exclude,omitempty"`
	Context      sourceContext `json:"context,omitempty"`
	MaxFiles     int           `json:"max_files,omitempty"`
	MaxMatches   int           `json:"max_matches,omitempty"`
	Offset       int64         `json:"offset,omitempty"`
	Limit        int           `json:"limit,omitempty"`
	Continuation string        `json:"continuation,omitempty"`
	StartLine    int           `json:"start_line,omitempty"`
	EndLine      int           `json:"end_line,omitempty"`
	LineNumbers  bool          `json:"line_numbers,omitempty"`
}

func (readFileHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "read",
		Description:         "Read and inspect workspace artifacts. Text keeps byte/line pagination and SHA-256 evidence; source performs bounded recursive repository search with context; image and structured views provide bounded pure-Go analysis.",
		Kind:                tool.KindForName("read"),
		Mutability:          tool.MutabilityReadOnly,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyWorkspaceRead},
		Evidence:            tool.EvidenceWorkspace,
		PermissionDetailKey: "path",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Workspace-relative path to read; use . only for directory-oriented source inspection. Absolute paths are outside the workspace",
				},
				"view": map[string]any{
					"type":        "string",
					"enum":        []string{"auto", "text", "source", "image", "structured", "metadata"},
					"description": "Artifact view; source performs bounded recursive repository inspection under a directory; auto preserves text-like files while inspecting supported images and binary metadata; structured analyzes JSON/JSONL/CSV/TSV",
				},

				"query":   map[string]any{"type": "string", "description": "Required for source view; literal text or regular expression to match"},
				"mode":    map[string]any{"type": "string", "enum": []string{"literal", "regex"}, "description": "Source match mode; defaults to literal"},
				"include": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Source include globs such as **/*.go"},
				"exclude": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Source exclude globs such as **/*_test.go"},
				"context": map[string]any{"type": "object", "properties": map[string]any{
					"before": map[string]any{"type": "integer", "minimum": 0, "maximum": maxSourceContext},
					"after":  map[string]any{"type": "integer", "minimum": 0, "maximum": maxSourceContext},
				}, "additionalProperties": false},
				"max_files":   map[string]any{"type": "integer", "minimum": 0, "maximum": maxSourceFiles, "description": "Source scan file cap; defaults to 10000"},
				"max_matches": map[string]any{"type": "integer", "minimum": 0, "maximum": maxSourceMatches, "description": "Source match cap; defaults to 100"},
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
					"minimum":     0,
					"maximum":     MaxReadFileBytes,
					"description": "Target page size in bytes; defaults to 2 MiB and may extend to finish one UTF-8 code point",
				},
				"start_line": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Optional 1-based first line to read; use with end_line for narrow source inspection",
				},
				"end_line": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Optional 1-based inclusive last line; 0 reads through EOF",
				},
				"line_numbers": map[string]any{
					"type":        "boolean",
					"description": "Prefix selected lines with their 1-based line number",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
	}
}
