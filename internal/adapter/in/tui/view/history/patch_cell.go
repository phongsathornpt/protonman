package history

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// PatchCell gives edit tools a change-oriented presentation. Paths are best
// effort presentation metadata derived from already-validated tool arguments;
// they never participate in authorization or execution.
type PatchCell struct {
	CallID      string
	Name        string
	Summary     string
	Paths       []string
	Body        string
	Running     bool
	Truncated   bool
	Denied      bool
	FailureCode tool.ErrorCode
	Spinner     string
}

func (PatchCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c PatchCell) RenderWidth(width int) []string {
	title := tool.DisplayName(c.Name)
	if strings.TrimSpace(c.Summary) != "" {
		title += " · " + c.Summary
	}
	var header string
	var headerStyle lipgloss.Style
	if c.Running {
		indicator := " …"
		if c.Spinner != "" {
			indicator = " " + c.Spinner
		}
		header = tuistyle.GlyphEdit + title + indicator
		headerStyle = tuistyle.PlanStyle
	} else if c.Denied {
		header = tuistyle.GlyphToolDenied + title + tuistyle.GlyphSep + "denied"
		headerStyle = tuistyle.WarningStyle
	} else if c.FailureCode != "" {
		header = tuistyle.GlyphToolError + title + tuistyle.GlyphSep + string(c.FailureCode)
		headerStyle = tuistyle.ErrorStyle
	} else {
		header = tuistyle.GlyphToolSuccess + title
		headerStyle = tuistyle.SuccessStyle
	}
	visiblePaths := c.Paths
	if len(visiblePaths) == 1 && strings.TrimSpace(visiblePaths[0]) != "" {
		header += " " + toolview.FormatPath(visiblePaths[0])
		visiblePaths = nil
	}
	out := make([]string, 0, 1)
	for _, line := range safeWrappedLines(header, max(1, width)) {
		out = append(out, headerStyle.Render(line))
	}
	hiddenPaths := 0
	if len(visiblePaths) > 4 {
		hiddenPaths = len(visiblePaths) - 3
		visiblePaths = visiblePaths[:3]
	}
	for _, path := range visiblePaths {
		styledPath := toolview.FormatPath(path)
		for _, wrapped := range wrapStyledLines(styledPath, max(1, width-2)) {
			out = append(out, "  "+wrapped)
		}
	}
	if hiddenPaths > 0 {
		out = append(out, tuistyle.ToolFoldStyle.Render(fmt.Sprintf("  … (+%d more files · ctrl+t for full list)", hiddenPaths)))
	}
	if !c.Running && c.Body != "" && (c.Denied || c.FailureCode != "" || len(c.Paths) == 0) {
		bodyLines := resultBodyLines(c.Body, nil, c.Truncated, c.Denied, c.FailureCode)
		if len(bodyLines) > 0 {
			folded := toolview.FormatOutputFold(bodyLines, 3)
			for _, line := range folded {
				if styled, isDiff := toolview.StyleDiffLine(line); isDiff {
					for _, wrapped := range safeWrappedLines(styled, max(1, width-2)) {
						out = append(out, "  "+wrapped)
					}
					continue
				}
				for _, wrapped := range safeWrappedLines(line, max(1, width-2)) {
					out = append(out, tuistyle.BodyStyle.Render("  "+wrapped))
				}
			}
		}
	}
	return out
}
func (c PatchCell) RawLines() []string {
	title := tool.DisplayName(c.Name)
	if strings.TrimSpace(c.Summary) != "" {
		title += " · " + c.Summary
	}
	out := []string{tuistyle.GlyphEdit + sanitizeBubbleText(title)}
	for _, path := range c.Paths {
		out = append(out, sanitizeBubbleText(path))
	}
	out = append(out, resultBodyLines(c.Body, nil, c.Truncated, c.Denied, c.FailureCode)...)
	return out
}
func (c PatchCell) LineCount() int           { return len(c.RawLines()) }
func (c PatchCell) historyToolID() string    { return c.CallID }
func (c PatchCell) historyToolName() string  { return c.Name }
func (c PatchCell) historyToolRunning() bool { return c.Running }
