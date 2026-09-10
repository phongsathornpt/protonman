package toolview

import (
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"strings"
)

// StyleDiffLine checks if a line looks like a diff line and applies syntax coloring.
func StyleDiffLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "+++ ") || strings.HasPrefix(trimmed, "--- ") {
		return tuistyle.MutedStyle.Bold(true).Render(line), true
	}
	if strings.HasPrefix(trimmed, "+") {
		return tuistyle.DiffAddStyle.Render(line), true
	}
	if strings.HasPrefix(trimmed, "-") {
		return tuistyle.DiffDeleteStyle.Render(line), true
	}
	if strings.HasPrefix(trimmed, "@@") {
		return tuistyle.DiffHunkStyle.Render(line), true
	}
	if strings.HasPrefix(trimmed, "diff --git ") || strings.HasPrefix(trimmed, "index ") {
		return tuistyle.MutedStyle.Bold(true).Render(line), true
	}
	return line, false
}
