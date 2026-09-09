package toolview

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

func summarizeReadFile(body string, truncated bool) string {
	return summarizeReadFileTarget("", body, truncated)
}

func summarizeReadFileTarget(target string, body string, truncated bool) string {
	if body == "" {
		return "0 B (empty file)"
	}
	if strings.HasPrefix(body, "image ") || strings.HasPrefix(body, "structured ") || strings.HasPrefix(body, "binary ") {
		return textview.TruncateEllipsis(body, 120)
	}
	sizeStr := formatByteSize(len(body))
	if truncated {
		sizeStr += "+ truncated"
	}
	lines := strings.Count(body, "\n") + 1
	var lineStr string
	if lines == 1 {
		lineStr = fmt.Sprintf("1 line (%s)", sizeStr)
	} else {
		lineStr = fmt.Sprintf("%d lines (%s)", lines, sizeStr)
	}
	if ft := detectFileType(target); ft != "" {
		return fmt.Sprintf("%s · %s", lineStr, ft)
	}
	return lineStr
}

// detectFileType returns a concise human-readable language or file format label.
func detectFileType(path string) string {
	clean := strings.ToLower(strings.TrimSpace(path))
	if clean == "" {
		return ""
	}
	clean = strings.Trim(clean, `"'`)
	ext := filepath.Ext(clean)
	switch ext {
	case ".go":
		return "Go"
	case ".json":
		return "JSON"
	case ".toml":
		return "TOML"
	case ".yaml", ".yml":
		return "YAML"
	case ".md", ".markdown":
		return "Markdown"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx":
		return "JavaScript"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".sh", ".bash", ".zsh":
		return "Shell"
	case ".sql":
		return "SQL"
	case ".html", ".htm":
		return "HTML"
	case ".css":
		return "CSS"
	case ".c", ".h":
		return "C"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "C++"
	case ".mod":
		return "Go Module"
	case ".sum":
		return "Go Sum"
	case ".proto":
		return "Protobuf"
	default:
		base := filepath.Base(clean)
		if base == "dockerfile" {
			return "Dockerfile"
		}
		if base == "makefile" {
			return "Makefile"
		}
		return ""
	}
}

// FormatPath visually differentiates directory path from base filename.
func FormatPath(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	// For grep query with " in ":
	if strings.Contains(target, " in ") {
		parts := strings.SplitN(target, " in ", 2)
		pattern := strings.Trim(parts[0], `"`)
		if ansi.StringWidth(pattern) > 30 {
			pattern = textview.TruncateEllipsis(pattern, 28)
		}
		return tuistyle.ToolTargetStyle.Render(fmt.Sprintf("%q", pattern)) + tuistyle.MutedStyle.Render(" in ") + FormatPath(parts[1])
	}
	// For quoted strings (e.g. web search query or grep pattern):
	if strings.HasPrefix(target, `"`) && strings.HasSuffix(target, `"`) {
		inner := strings.Trim(target, `"`)
		if ansi.StringWidth(inner) > 36 {
			inner = textview.TruncateEllipsis(inner, 34)
			return tuistyle.ToolTargetStyle.Render(fmt.Sprintf("%q", inner))
		}
		return tuistyle.ToolTargetStyle.Render(target)
	}
	// Check if URL:
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		if ansi.StringWidth(target) > 50 {
			target = textview.TruncateEllipsis(target, 48)
		}
		return tuistyle.ToolTargetStyle.Render(target)
	}

	idx := strings.LastIndex(target, "/")
	if idx == -1 {
		idx = strings.LastIndex(target, `\`)
	}
	if idx == -1 {
		return tuistyle.ToolTargetStyle.Render(target)
	}

	dir := target[:idx+1]
	base := target[idx+1:]
	return tuistyle.ToolDirStyle.Render(dir) + tuistyle.ToolTargetStyle.Render(base)
}

// FormatPathWidth renders a target within a fixed terminal-cell budget, preserving path suffixes.
func FormatPathWidth(target string, width int) string {
	target = strings.TrimSpace(target)
	if target == "" || width <= 0 {
		return ""
	}
	if ansi.StringWidth(target) <= width {
		return FormatPath(target)
	}
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
		(strings.HasPrefix(target, `"`) && strings.HasSuffix(target, `"`)) || strings.Contains(target, " in ") {
		return tuistyle.ToolTargetStyle.Render(textview.TruncateEllipsis(target, width))
	}
	idx := strings.LastIndexAny(target, `/\\`)
	if idx < 0 {
		return tuistyle.ToolTargetStyle.Render(textview.TruncateLeftEllipsis(target, width))
	}
	base := target[idx+1:]
	if base == "" {
		return tuistyle.ToolDirStyle.Render(textview.TruncateLeftEllipsis(target, width))
	}
	if ansi.StringWidth(base) >= width-2 {
		return tuistyle.ToolTargetStyle.Render(textview.TruncateLeftEllipsis(base, width))
	}
	return tuistyle.ToolDirStyle.Render("…/") + tuistyle.ToolTargetStyle.Render(base)
}

// extractReadFileExcerpt extracts a 1-line structural header preview (e.g. package, shebang, title).
func ExtractReadFileExcerpt(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		// Structural headers only (avoid printing arbitrary code statements)
		if strings.HasPrefix(trimmed, "package ") ||
			strings.HasPrefix(trimmed, "#!/") ||
			strings.HasPrefix(trimmed, "# ") ||
			strings.HasPrefix(trimmed, "## ") ||
			strings.HasPrefix(trimmed, "module ") {
			return textview.TruncateEllipsis(trimmed, 65)
		}
	}
	return ""
}

func summarizeListDir(body string, truncated bool) string {
	if body == "" {
		return "empty directory"
	}
	lines := strings.Split(body, "\n")
	dirs, files := 0, 0
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "dir ") {
			dirs++
		} else {
			files++
		}
	}
	total := dirs + files
	if total == 0 {
		return "empty directory"
	}
	suffix := ""
	if truncated {
		suffix = " + truncated"
	}
	return fmt.Sprintf("%d items (%d dirs, %d files)%s", total, dirs, files, suffix)
}
