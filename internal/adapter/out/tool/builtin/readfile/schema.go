package readfile

import "github.com/phongsathornpt/protonman/internal/core/tool"

const MaxReadFileBytes = 2 * 1024 * 1024

type readFileInput struct {
	Path         string `json:"path"`
	View         string `json:"view,omitempty"`
	Offset       int64  `json:"offset,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	Continuation string `json:"continuation,omitempty"`
	StartLine    int    `json:"start_line,omitempty"`
	EndLine      int    `json:"end_line,omitempty"`
	LineNumbers  bool   `json:"line_numbers,omitempty"`
}

func (readFileHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "read",
		Description:         "Read and inspect workspace artifacts. Text keeps byte/line pagination and SHA-256 evidence; image and structured views provide bounded pure-Go analysis.",
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
					"enum":        []string{"auto", "text", "image", "structured", "metadata"},
					"description": "Artifact view; auto preserves text-like files while inspecting supported images and binary metadata; structured analyzes JSON/JSONL/CSV/TSV",
				},

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
