package history

import (
	"fmt"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// ToolCell is the generic representation for a tool that has no specialized
// presentation model.
type ToolCell struct {
	CallID          string
	Name            string
	Body            string
	Stdout          string
	Stderr          string
	Running         bool
	ExitCode        *int
	Truncated       bool
	StdoutTruncated bool
	StderrTruncated bool
	Denied          bool
	FailureCode     tool.ErrorCode
	Spinner         string
	Target          string
	ToolKind        tool.Kind
	Summary         string
	ShowDetail      bool
}

func (ToolCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c ToolCell) RenderWidth(width int) []string {
	var headerLine string

	if c.Running {
		glyph := toolview.KindGlyph(c.ToolKind, c.Name)
		indicator := " …"
		if c.Spinner != "" {
			indicator = " " + c.Spinner
		}
		targetStr := ""
		if strings.TrimSpace(c.Target) != "" {
			targetStr = " " + toolview.FormatPath(c.Target)
		}
		headerLine = tuistyle.ToolStyle.Render(glyph) + tuistyle.MutedStyle.Render(sanitizeBubbleText(tool.DisplayName(c.Name))) + targetStr + tuistyle.ToolStyle.Render(indicator)
	} else if c.Denied {
		targetStr := ""
		if strings.TrimSpace(c.Target) != "" {
			targetStr = " " + toolview.FormatPath(c.Target)
		}
		headerLine = tuistyle.WarningStyle.Render(tuistyle.GlyphToolDenied) + tuistyle.MutedStyle.Render(sanitizeBubbleText(tool.DisplayName(c.Name))) + targetStr + tuistyle.WarningStyle.Render(tuistyle.GlyphSep+"denied")
	} else if c.FailureCode != "" {
		targetStr := ""
		if strings.TrimSpace(c.Target) != "" {
			targetStr = " " + toolview.FormatPath(c.Target)
		}
		headerLine = tuistyle.ErrorStyle.Render(tuistyle.GlyphToolError) + tuistyle.MutedStyle.Render(sanitizeBubbleText(tool.DisplayName(c.Name))) + targetStr + tuistyle.ErrorStyle.Render(tuistyle.GlyphSep+string(c.FailureCode))
	} else if strings.TrimSpace(c.Name) == tool.NameSkill {
		target := c.Target
		if target == "" {
			if skillName := toolview.ExtractSkillContentName(c.Body); skillName != "" {
				target = fmt.Sprintf("%q", skillName)
			}
		}
		if target != "" {
			headerLine = tuistyle.SuccessStyle.Render(tuistyle.GlyphToolSuccess) + tuistyle.MutedStyle.Render("Activated skill ") + tuistyle.ToolTargetStyle.Render(target)
		} else {
			headerLine = tuistyle.SuccessStyle.Render(tuistyle.GlyphToolSuccess) + tuistyle.MutedStyle.Render(sanitizeBubbleText(tool.DisplayName(c.Name)))
		}
	} else {
		summary := c.Summary
		if summary == "" && c.Body != "" {
			summary = toolview.SummarizeOutput(c.Name, c.ToolKind, c.Target, c.Body, c.ExitCode, c.Truncated)
		}
		targetStr := ""
		if strings.TrimSpace(c.Target) != "" {
			targetStr = " " + toolview.FormatPath(c.Target)
		}
		summaryStr := ""
		if summary != "" {
			summaryStr = tuistyle.ToolSummaryStyle.Render(tuistyle.GlyphSep + summary)
		}
		headerLine = tuistyle.SuccessStyle.Render(tuistyle.GlyphToolSuccess) + tuistyle.MutedStyle.Render(sanitizeBubbleText(tool.DisplayName(c.Name))) + targetStr + summaryStr
	}

	out := make([]string, 0, 1)
	for _, line := range wrapStyledLines(headerLine, max(1, width)) {
		out = append(out, line)
	}

	showDetail := !c.Running && (c.ShowDetail || c.Denied || c.FailureCode != "")
	if showDetail {
		if c.ShowDetail && strings.TrimSpace(c.Name) == tool.NameRead && !c.Denied && c.FailureCode == "" && c.Body != "" {
			if excerpt := toolview.ExtractReadFileExcerpt(c.Body); excerpt != "" {
				out = append(out, tuistyle.ToolExcerptStyle.Render("  ↳ "+excerpt))
			}
		}
		if c.ToolKind == tool.KindGrep || c.Name == "grep" {
			for _, line := range toolview.FormatGrepView(c.bodyLines(), c.Target, width) {
				out = append(out, "  "+line)
			}
		} else {
			bodyLines := c.bodyLines()
			if len(bodyLines) > 0 {
				folded := toolview.FormatOutputFold(bodyLines, 3)
				for _, line := range folded {
					for _, wrapped := range wrapStyledLines(line, max(1, width-2)) {
						out = append(out, tuistyle.BodyStyle.Render("  "+wrapped))
					}
				}
			}
		}
	}

	return out
}
func routineToolAggregationKey(cell HistoryCell) string {
	c, ok := cell.(*ToolCell)
	if !ok || c.Running || c.Denied || c.FailureCode != "" || c.ShowDetail {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(c.Name)) {
	case "read":
		return "read"
	case "ls", "list":
		return "list"
	case "find", "grep", "search":
		return "search"
	default:
		return ""
	}
}

func renderRoutineToolAggregate(key string, count, width int) []string {
	if count < 2 {
		return nil
	}
	label := "Operations"
	switch key {
	case "read":
		label = fmt.Sprintf("Read %d files", count)
	case "list":
		label = fmt.Sprintf("Listed %d locations", count)
	case "search":
		label = fmt.Sprintf("Search %d queries", count)
	}
	line := tuistyle.SuccessStyle.Render(tuistyle.GlyphToolSuccess) + tuistyle.MutedStyle.Render(label)
	return wrapStyledLines(line, max(1, width))
}

func (c ToolCell) RawLines() []string {
	header := sanitizeBubbleText(tool.DisplayName(c.Name))
	if c.Target != "" {
		header += " " + sanitizeBubbleText(c.Target)
	}
	out := []string{header}
	out = append(out, c.bodyLines()...)
	return out
}
func (c ToolCell) LineCount() int           { return len(c.RawLines()) }
func (c ToolCell) historyToolID() string    { return c.CallID }
func (c ToolCell) historyToolName() string  { return c.Name }
func (c ToolCell) historyToolRunning() bool { return c.Running }
func (c ToolCell) bodyLines() []string {
	if strings.TrimSpace(c.Name) == tool.NameSkill {
		return formatSkillToolBody(c.Body, c.ExitCode, c.Truncated, c.Denied, c.FailureCode)
	}
	return resultBodyLines(c.Body, c.ExitCode, c.Truncated, c.Denied, c.FailureCode)
}

func formatSkillToolBody(body string, exitCode *int, truncated bool, denied bool, failureCode tool.ErrorCode) []string {
	if denied {
		return []string{"denied"}
	}
	if failureCode != "" {
		return []string{"failure: " + string(failureCode)}
	}
	if skillName := toolview.ExtractSkillContentName(body); skillName != "" {
		return []string{fmt.Sprintf("[x] Activated skill %q", skillName)}
	}
	return resultBodyLines(body, exitCode, truncated, denied, failureCode)
}
