package history

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/execview"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// AgentToolCell renders subagent lifecycle operations as orchestration rather
// than generic tool RPCs.
type AgentToolCell struct {
	CallID  string
	Name    string
	Target  string
	Summary string
	Running bool
	Spinner string
}

func (AgentToolCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c AgentToolCell) Render() []string    { return c.RenderWidth(defaultHistoryWidth) }
func (c AgentToolCell) RenderWidth(width int) []string {
	label := c.presentationLabel(true)
	if c.Running {
		return wrapStyledLines(tuistyle.ToolStyle.Render(tuistyle.GlyphAgent)+tuistyle.MutedStyle.Render(sanitizeBubbleText(label)), max(1, width))
	}
	return wrapStyledLines(tuistyle.SuccessStyle.Render(tuistyle.GlyphToolSuccess)+tuistyle.MutedStyle.Render(sanitizeBubbleText(label)), max(1, width))
}
func (c AgentToolCell) presentationLabel(includeSpinner bool) string {
	label := c.Summary
	if c.Running {
		switch c.Name {
		case "wait_agent":
			label = "Waiting for " + c.Target
		case "cancel_agent":
			label = "Canceling " + c.Target
		case "get_agent":
			label = "Checking " + c.Target
		case "list_agents":
			label = "Checking subagents"
		default:
			label = "Coordinating subagents"
		}
		if includeSpinner && c.Spinner != "" {
			label = c.Spinner + " " + label
		}
	}
	if label == "" {
		label = tool.DisplayName(c.Name)
	}
	return label
}
func (c AgentToolCell) RawLines() []string {
	return []string{sanitizeBubbleText(c.presentationLabel(false))}
}
func (c AgentToolCell) LineCount() int           { return 1 }
func (c AgentToolCell) historyToolID() string    { return c.CallID }
func (c AgentToolCell) historyToolName() string  { return c.Name }
func (c AgentToolCell) historyToolRunning() bool { return c.Running }

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
}

func (ToolCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c ToolCell) Render() []string    { return c.RenderWidth(defaultHistoryWidth) }
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
	} else if c.Name == "skill" || c.Name == "activate_skill" {
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

	// Read file excerpt preview
	if !c.Running && tool.CanonicalName(c.Name) == "read" && !c.Denied && c.FailureCode == "" && c.Body != "" {
		if excerpt := toolview.ExtractReadFileExcerpt(c.Body); excerpt != "" {
			out = append(out, tuistyle.ToolExcerptStyle.Render("  ↳ "+excerpt))
		}
	}

	if !c.Running && !toolview.ShouldSuppressBody(c.ToolKind, c.Name) {
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
	if c.Name == "skill" || c.Name == "activate_skill" {
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

// ExecCell gives shell execution a compact, command-oriented presentation.
type ExecCell struct {
	CallID          string
	Name            string
	Command         string
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
	StartedAt       time.Time
	Duration        time.Duration
}

func (ExecCell) Kind() HistoryCellKind { return HistoryCellTool }
func (c ExecCell) Render() []string    { return c.RenderWidth(defaultHistoryWidth) }
func (c ExecCell) RenderWidth(width int) []string {
	command := strings.TrimSpace(c.Command)
	if command == "" {
		command = c.Name
	}
	presentation := c.presentation(command)
	failed := c.FailureCode != "" || (c.ExitCode != nil && *c.ExitCode != 0)
	if !failed && !c.Denied && presentation.Summary == "" && presentation.SuccessSummary != "" {
		presentation.Summary = presentation.SuccessSummary
	}

	width = max(1, width)
	var out []string
	if c.Running {
		indicator := " …"
		if c.Spinner != "" {
			indicator = " " + c.Spinner
		}
		header := tuistyle.CommandStyle.Render(sanitizeBubbleText(presentation.Title + indicator))
		out = append(out, wrapStyledLines(header, width)...)
	} else {
		title := sanitizeBubbleText(presentation.Title)
		if title == "" {
			title = "$ " + sanitizeBubbleText(command)
		}
		var glyph string
		switch {
		case c.Denied:
			glyph = tuistyle.WarningStyle.Render(tuistyle.GlyphToolDenied)
		case failed:
			glyph = tuistyle.ErrorStyle.Render(tuistyle.GlyphToolError)
		default:
			glyph = tuistyle.SuccessStyle.Render(tuistyle.GlyphToolSuccess)
		}
		header := glyph + tuistyle.CommandStyle.Render(title)
		summary := presentation.Summary
		if c.Denied {
			summary = "denied"
		} else if failed && summary == "" {
			switch {
			case c.ExitCode != nil:
				summary = fmt.Sprintf("exit %d", *c.ExitCode)
			case c.FailureCode != "":
				summary = string(c.FailureCode)
			default:
				summary = "failed"
			}
		}

		if summary == "" && c.Duration > 0 {
			header = alignExecDuration(header, execview.FormatDuration(c.Duration), width)
		}
		out = append(out, wrapStyledLines(header, width)...)
		if summary != "" {
			out = append(out, renderExecMetaLine(summary, c.Duration, width))
		}
	}

	if !c.Running {
		contentWidth := max(20, width-4)
		for _, line := range c.renderOutputLines() {
			style := tuistyle.BodyStyle
			if strings.TrimSpace(line) == "stderr:" {
				style = tuistyle.WarningStyle
			}
			clean := line
			isFoldIndicator := strings.HasPrefix(clean, "… (")
			if !isFoldIndicator && ansi.StringWidth(clean) > contentWidth {
				clean = textview.TruncateEllipsis(clean, contentWidth)
			}
			if styled, isDiff := toolview.StyleDiffLine(clean); isDiff {
				for _, wrapped := range safeWrappedLines(styled, max(1, width-2)) {
					out = append(out, "  "+wrapped)
				}
				continue
			}
			for _, wrapped := range safeWrappedLines(clean, max(1, width-2)) {
				out = append(out, style.Render("  "+wrapped))
			}
		}
	}
	return out
}

func alignExecDuration(header, duration string, width int) string {
	if duration == "" {
		return header
	}
	gap := width - ansi.StringWidth(header) - ansi.StringWidth(duration)
	if gap < 2 {
		return header + tuistyle.ToolSummaryStyle.Render(tuistyle.GlyphSep+duration)
	}
	return header + strings.Repeat(" ", gap) + tuistyle.ToolSummaryStyle.Render(duration)
}

func renderExecMetaLine(summary string, duration time.Duration, width int) string {
	text := sanitizeBubbleText(strings.TrimSpace(summary))
	durationText := ""
	if duration > 0 {
		durationText = execview.FormatDuration(duration)
	}
	if durationText == "" {
		return tuistyle.ToolSummaryStyle.Render("  " + text)
	}
	maxSummaryWidth := max(1, width-2-ansi.StringWidth(durationText)-2)
	if ansi.StringWidth(text) > maxSummaryWidth {
		text = textview.TruncateEllipsis(text, maxSummaryWidth)
	}
	left := "  " + text
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(durationText)
	if gap < 2 {
		gap = 2
	}
	return tuistyle.ToolSummaryStyle.Render(left) + strings.Repeat(" ", gap) + tuistyle.ToolSummaryStyle.Render(durationText)
}

func (c ExecCell) presentation(command string) execview.Presentation {
	stdout, stderr := c.Stdout, c.Stderr
	if stdout == "" && stderr == "" {
		stdout = c.Body
	}
	return execview.Present(command, stdout, stderr)
}

func (c ExecCell) RawLines() []string {
	command := strings.TrimSpace(c.Command)
	if command == "" {
		command = c.Name
	}
	out := []string{"$ " + sanitizeBubbleText(command)}
	out = append(out, c.outputLines(true)...)
	return out
}
func (c ExecCell) renderOutputLines() []string {
	command := strings.TrimSpace(c.Command)
	if command == "" {
		command = c.Name
	}
	presentation := c.presentation(command)
	failed := c.Denied || c.FailureCode != "" || (c.ExitCode != nil && *c.ExitCode != 0)
	if !failed && presentation.SuppressRaw {
		return append([]string(nil), presentation.Details...)
	}
	structured := c.Stdout != "" || c.Stderr != "" || c.StdoutTruncated || c.StderrTruncated
	if !structured {
		return toolview.FormatOutputFold(resultBodyLines(c.Body, nil, c.Truncated, false, ""), 3)
	}
	out := make([]string, 0, 8)
	stdoutLines := rawTextLines(strings.TrimRight(c.Stdout, "\n"))
	if c.StdoutTruncated {
		stdoutLines = append(stdoutLines, "stdout truncated")
	}
	out = append(out, toolview.FormatOutputFold(stdoutLines, 3)...)
	if c.Stderr != "" || c.StderrTruncated {
		out = append(out, "stderr:")
		stderrLines := rawTextLines(strings.TrimRight(c.Stderr, "\n"))
		if c.StderrTruncated {
			stderrLines = append(stderrLines, "stderr truncated")
		}
		out = append(out, toolview.FormatOutputFold(stderrLines, 3)...)
	}
	if c.Truncated && !c.StdoutTruncated && !c.StderrTruncated {
		out = append(out, "output truncated")
	}
	return out
}

func (c ExecCell) outputLines(includeStatus bool) []string {
	structured := c.Stdout != "" || c.Stderr != "" || c.StdoutTruncated || c.StderrTruncated
	if !structured {
		exitCode := c.ExitCode
		denied := c.Denied
		failureCode := c.FailureCode
		if !includeStatus {
			exitCode, denied, failureCode = nil, false, ""
		}
		return resultBodyLines(c.Body, exitCode, c.Truncated, denied, failureCode)
	}
	lines := make([]string, 0, strings.Count(c.Stdout, "\n")+strings.Count(c.Stderr, "\n")+6)
	if stdout := strings.TrimRight(c.Stdout, "\n"); stdout != "" {
		lines = append(lines, rawTextLines(stdout)...)
	}
	if c.StdoutTruncated {
		lines = append(lines, "stdout truncated")
	}
	if stderr := strings.TrimRight(c.Stderr, "\n"); stderr != "" {
		lines = append(lines, "stderr:")
		for _, line := range rawTextLines(stderr) {
			lines = append(lines, "  "+line)
		}
	}
	if c.StderrTruncated {
		lines = append(lines, "stderr truncated")
	}
	if c.Truncated && !c.StdoutTruncated && !c.StderrTruncated {
		lines = append(lines, "output truncated")
	}
	if includeStatus {
		if c.ExitCode != nil {
			lines = append(lines, fmt.Sprintf("exit %d", *c.ExitCode))
		}
		if c.Denied {
			lines = append(lines, "denied")
		}
		if c.FailureCode != "" && !(c.ExitCode != nil && c.FailureCode == tool.ErrorCodeCommandFailed) {
			lines = append(lines, "failure: "+string(c.FailureCode))
		}
	}
	return lines
}

func (c ExecCell) LineCount() int           { return len(c.RawLines()) }
func (c ExecCell) historyToolID() string    { return c.CallID }
func (c ExecCell) historyToolName() string  { return c.Name }
func (c ExecCell) historyToolRunning() bool { return c.Running }

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
func (c PatchCell) Render() []string    { return c.RenderWidth(defaultHistoryWidth) }
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
	out := make([]string, 0, 1)
	for _, line := range safeWrappedLines(header, max(1, width)) {
		out = append(out, headerStyle.Render(line))
	}
	visiblePaths := c.Paths
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

func resultBodyLines(body string, exitCode *int, truncated bool, denied bool, failureCode tool.ErrorCode) []string {
	body = strings.TrimRight(body, "\n")
	parts := make([]string, 0, strings.Count(body, "\n")+4)
	if body != "" {
		parts = append(parts, rawTextLines(body)...)
	}
	if exitCode != nil {
		parts = append(parts, fmt.Sprintf("exit %d", *exitCode))
	}
	if truncated {
		parts = append(parts, "output truncated")
	}
	if denied {
		parts = append(parts, "denied")
	}
	if failureCode != "" {
		parts = append(parts, "failure: "+string(failureCode))
	}
	return parts
}
