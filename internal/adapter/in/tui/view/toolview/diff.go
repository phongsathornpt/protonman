package toolview

import (
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/base/diffutil"
)

// StyleDiffLine checks if a line looks like a diff line and applies syntax coloring.
func StyleDiffLine(line string) (string, bool) {
	if line == "" {
		return line, false
	}
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
		return tuistyle.MutedStyle.Bold(true).Render(line), true
	}
	if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
		return tuistyle.DiffAddStyle.Render(line), true
	}
	if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
		return tuistyle.DiffDeleteStyle.Render(line), true
	}
	if strings.HasPrefix(line, "@@") || strings.HasPrefix(trimmed, "@@") {
		return tuistyle.DiffHunkStyle.Render(line), true
	}
	if strings.HasPrefix(trimmed, "diff --git ") || strings.HasPrefix(trimmed, "index ") ||
		strings.HasPrefix(trimmed, "new file mode ") || strings.HasPrefix(trimmed, "deleted file mode ") ||
		strings.HasPrefix(trimmed, "similarity index ") || strings.HasPrefix(trimmed, "rename from ") ||
		strings.HasPrefix(trimmed, "rename to ") || strings.HasPrefix(trimmed, "Binary files ") {
		return tuistyle.MutedStyle.Bold(true).Render(line), true
	}
	if strings.HasPrefix(trimmed, "\\ ") || trimmed == "\\ No newline at end of file" {
		return tuistyle.MutedStyle.Render(line), true
	}
	return line, false
}

// ExtractDiffPreview extracts up to maxLines from a unified diff text starting at the first hunk header,
// returning the preview lines and the count of omitted diff lines.
func ExtractDiffPreview(diffText string, maxLines int) ([]string, int) {
	return diffutil.ExtractPreview(diffText, maxLines)
}
