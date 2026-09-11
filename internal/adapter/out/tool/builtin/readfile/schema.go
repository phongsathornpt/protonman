package readfile

import "github.com/phongsathornpt/protonman/internal/core/tool"

const (
	DefaultReadFileBytes = 64 * 1024
	MaxReadFileBytes     = 2 * 1024 * 1024
)

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
		Name:                tool.NameRead,
		Description:         "Read a known workspace artifact. Text supports byte pagination or line selection; line selectors take precedence if both modes are supplied. Image, structured, and metadata views provide bounded inspection.",
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
					"description": "Workspace-relative artifact path. Use ls/find/grep to discover files instead of guessing filenames; absolute paths are outside the workspace",
				},
				"view": map[string]any{
					"type":        "string",
					"enum":        []string{"auto", "text", "image", "structured", "metadata"},
					"default":     "auto",
					"description": "Inspection mode; auto is the default, text reads UTF-8 content, image analyzes supported images, structured analyzes JSON/JSONL/CSV/TSV, metadata reports artifact type and size",
				},

				"offset": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Text-only byte offset; use next_offset from a truncated text result. Ignored when line selection is requested",
				},
				"continuation": map[string]any{
					"type":        "string",
					"description": "Text-only snapshot token from a truncated result; send it with next_offset to detect file changes. Ignored when line selection is requested",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"maximum":     MaxReadFileBytes,
					"description": "Text-only output page size; defaults to 64 KiB, may be increased up to 2 MiB, and may extend to finish one UTF-8 code point",
				},
				"start_line": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Text-only 1-based first line; selects line mode and takes precedence over byte offset/continuation",
				},
				"end_line": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Text-only 1-based inclusive last line; 0 reads through EOF. Selects line mode and takes precedence over byte offset/continuation",
				},
				"line_numbers": map[string]any{
					"type":        "boolean",
					"description": "Text-only; prefix selected lines with their 1-based line number. Enables line mode and takes precedence over byte offset/continuation",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
	}
}
