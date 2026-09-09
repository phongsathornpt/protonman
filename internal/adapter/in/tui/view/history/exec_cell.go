package history

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/execview"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

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

		if !failed && !c.Denied {
			meta := strings.TrimSpace(summary)
			if duration := execview.FormatDuration(c.Duration); duration != "" {
				if meta != "" {
					meta += tuistyle.GlyphSep + duration
				} else {
					meta = duration
				}
			}
			if meta != "" {
				header += tuistyle.ToolSummaryStyle.Render(tuistyle.GlyphSep + meta)
			}
			out = append(out, wrapStyledLines(header, width)...)
		} else {
			out = append(out, wrapStyledLines(header, width)...)
			if summary != "" {
				out = append(out, renderExecMetaLine(summary, c.Duration, width))
			}
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
