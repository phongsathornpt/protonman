package toolview

import (
	"encoding/json"
	"fmt"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// ExtractTarget inspects tool arguments and returns a human-facing target
// string (e.g. URL, filepath, pattern, command) and the normalized tool.Kind.
func ExtractTarget(name string, kind tool.Kind, args json.RawMessage) (string, tool.Kind) {
	call := tool.Call{Name: name, Arguments: args}
	target := call.Target()
	if kind == "" {
		kind = tool.KindForName(name)
	}
	return target, kind
}

// KindGlyph returns the appropriate category glyph for a tool.
func KindGlyph(kind tool.Kind, name string) string {
	switch kind {
	case tool.KindWeb:
		return tuistyle.GlyphWeb
	case tool.KindRead:
		if strings.TrimSpace(name) == tool.NameLS {
			return tuistyle.GlyphDir
		}
		return tuistyle.GlyphRead
	case tool.KindGrep:
		return tuistyle.GlyphSearch
	case tool.KindGit:
		return "⌥ "
	case tool.KindBash:
		return tuistyle.GlyphExec
	case tool.KindEdit:
		return tuistyle.GlyphEdit
	case tool.KindTask:
		return tuistyle.GlyphTodoActive
	case tool.KindAgent:
		return tuistyle.GlyphAgent
	}

	if strings.TrimSpace(name) == tool.NameSkill {
		return tuistyle.GlyphSkill
	}
	return tuistyle.GlyphGeneric
}

// SummarizeOutput produces a concise, high-signal semantic analysis summary
// from a tool's output body rather than dumping raw payloads into the TUI.
// ExtractSkillContentName extracts the activated skill name from structured skill output.
func ExtractSkillContentName(body string) string {
	for _, quote := range []string{`name="`, `name='`} {
		idx := strings.Index(body, quote)
		if idx == -1 {
			continue
		}
		rest := body[idx+len(quote):]
		if end := strings.IndexAny(rest, `"'`); end != -1 {
			return rest[:end]
		}
	}
	return ""
}

func SummarizeOutput(name string, kind tool.Kind, target string, body string, exitCode *int, truncated bool) string {
	bodyTrimmed := strings.TrimSpace(body)

	switch kind {
	case tool.KindWeb:
		if strings.TrimSpace(name) == tool.NameWeb && strings.HasPrefix(strings.TrimSpace(target), `"`) {
			return summarizeWebSearch(bodyTrimmed)
		}
		return summarizeWebFetch(bodyTrimmed, truncated)
	case tool.KindRead:
		if strings.TrimSpace(name) == tool.NameLS {
			return summarizeListDir(bodyTrimmed, truncated)
		}
		return summarizeReadFileTarget(target, bodyTrimmed, truncated)
	case tool.KindGrep:
		return summarizeGrep(bodyTrimmed, truncated)
	case tool.KindGit:
		return summarizeGitStatus(bodyTrimmed)
	case tool.KindEdit:
		return summarizeEdit(name, bodyTrimmed)
	case tool.KindTask:
		if strings.TrimSpace(name) == tool.NameTodo {
			var payload map[string]any
			if json.Unmarshal([]byte(bodyTrimmed), &payload) == nil {
				if _, ok := payload["items"]; ok {
					return summarizeTodoSnapshot(bodyTrimmed)
				}
			}
		}
		return summarizeTodoUpdate(bodyTrimmed)
	case tool.KindAgent:
		return summarizeAgentTool(name, bodyTrimmed)
	case tool.KindBash:
		if exitCode != nil {
			return fmt.Sprintf("exit %d", *exitCode)
		}
	}

	if strings.TrimSpace(name) == tool.NameSkill {
		if skillName := ExtractSkillContentName(body); skillName != "" {
			return fmt.Sprintf("Activated skill %q", skillName)
		}
		return "activated"
	}

	// Fallback for MCP or generic tools
	if bodyTrimmed == "" {
		return "completed"
	}
	lines := strings.Count(bodyTrimmed, "\n") + 1
	if lines > 1 {
		return fmt.Sprintf("%d lines (%s)", lines, formatByteSize(len(bodyTrimmed)))
	}
	return formatByteSize(len(bodyTrimmed))
}

// ShouldSuppressBody returns true if raw tool body dumping should be suppressed
// in the primary conversation viewport because the semantic header already summarizes it.
func ShouldSuppressBody(kind tool.Kind, name string) bool {
	switch kind {
	case tool.KindWeb, tool.KindRead, tool.KindGit, tool.KindAgent, tool.KindTask, tool.KindEdit:
		return true
	}
	switch strings.TrimSpace(name) {
	case "skill", "subagent", "todo", "edit":
		return true
	default:
		return false
	}
}

// FormatOutputFold folds long text outputs with a fold hint.
func FormatOutputFold(lines []string, maxVisible int) []string {
	if len(lines) <= maxVisible {
		return lines
	}
	if maxVisible < 2 {
		maxVisible = 2
	}
	headCount := maxVisible - 1
	hidden := len(lines) - maxVisible
	out := make([]string, 0, maxVisible+1)
	out = append(out, lines[:headCount]...)
	lineWord := "lines"
	if hidden == 1 {
		lineWord = "line"
	}
	foldMsg := tuistyle.ToolFoldStyle.Render(fmt.Sprintf("… (%d %s hidden · ctrl+t for full output)", hidden, lineWord))
	out = append(out, foldMsg)
	out = append(out, lines[len(lines)-1])
	return out
}

func formatByteSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024.0)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024.0*1024.0))
}
