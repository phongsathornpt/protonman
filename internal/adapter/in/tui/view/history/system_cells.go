package history

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/diagnostic"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// SystemCell renders low-priority status/history information.
type SystemCell struct{ Text string }

func (SystemCell) Kind() HistoryCellKind { return HistoryCellSystem }
func (c SystemCell) Render() []string    { return c.RenderWidth(defaultHistoryWidth) }
func (c SystemCell) RenderWidth(width int) []string {
	return styledWrappedLines(c.Text, width, tuistyle.MutedStyle)
}
func (c SystemCell) RawLines() []string { return rawTextLines(c.Text) }
func (c SystemCell) LineCount() int     { return len(c.RawLines()) }

// ErrorCell renders a failed operation or classified OpenCode error.
type ErrorCell struct {
	Title       string
	Text        string
	Code        tool.ErrorCode
	ErrorKind   diagnostic.Kind
	Badge       string
	Suggestions []string
	RawDetails  string
	Retryable   bool
}

func (ErrorCell) Kind() HistoryCellKind { return HistoryCellError }
func (c ErrorCell) Render() []string    { return c.RenderWidth(defaultHistoryWidth) }

func (c ErrorCell) RenderWidth(width int) []string {
	if width <= 0 {
		width = defaultHistoryWidth
	}

	if c.ErrorKind == diagnostic.KindToolFailed {
		return c.renderCompactToolFailure(width)
	}

	// Non-tool diagnostics keep the bordered card because they may contain
	// provider/account guidance that benefits from stronger visual grouping.
	if c.Badge != "" || len(c.Suggestions) > 0 || (c.ErrorKind != "" && c.ErrorKind != diagnostic.KindGeneric) {
		return c.renderCard(width)
	}

	// Simple fallback rendering for legacy or simple tool errors
	text := c.Text
	if c.Title != "" {
		text = c.Title + ": " + text
	}
	return styledWrappedLines(tuistyle.GlyphToolError+text, width, tuistyle.ErrorStyle)
}

func (c ErrorCell) renderCompactToolFailure(width int) []string {
	badge := c.Badge
	if badge == "" && c.Code != "" {
		badge = string(c.Code)
	}
	title := c.Title
	if title == "" {
		title = "tool"
	}

	header := tuistyle.GlyphToolError
	if badge != "" {
		header += "[" + badge + "] "
	}
	header += title
	if text := strings.TrimSpace(c.Text); text != "" {
		header += ": " + text
	}

	lines := styledWrappedLines(header, width, tuistyle.ErrorStyle)
	for _, suggestion := range c.Suggestions {
		lines = append(lines, styledWrappedLines("→ "+suggestion, width, tuistyle.MutedStyle)...)
	}
	return lines
}

func (c ErrorCell) renderCard(width int) []string {
	cardWidth := max(24, width-2)
	innerWidth := cardWidth - 4 // Account for border (2) and padding (2)

	badge := c.Badge
	if badge == "" {
		badge = "ERROR"
	}
	title := c.Title
	if title == "" {
		title = "Error"
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(tuistyle.AccentError).
		Render(fmt.Sprintf("%s[%s] %s", tuistyle.GlyphToolError, badge, title))

	bodyLines := safeWrappedLines(c.Text, innerWidth)
	cardContent := []string{header}
	if len(bodyLines) > 0 {
		cardContent = append(cardContent, "")
		for _, bLine := range bodyLines {
			cardContent = append(cardContent, tuistyle.BodyStyle.Render(bLine))
		}
	}

	if len(c.Suggestions) > 0 {
		cardContent = append(cardContent, "")
		for _, s := range c.Suggestions {
			wrappedS := safeWrappedLines("→ "+s, innerWidth)
			for _, w := range wrappedS {
				cardContent = append(cardContent, tuistyle.MutedStyle.Render(w))
			}
		}
	}

	joined := strings.Join(cardContent, "\n")
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tuistyle.AccentError).
		Padding(0, 1).
		Width(cardWidth)

	rendered := cardStyle.Render(joined)
	return strings.Split(rendered, "\n")
}

func (c ErrorCell) RawLines() []string {
	badge := c.Badge
	if badge == "" {
		badge = "ERROR"
	}
	title := c.Title
	if title == "" {
		title = "Error"
	}

	var lines []string
	if c.Badge != "" || len(c.Suggestions) > 0 || (c.ErrorKind != "" && c.ErrorKind != diagnostic.KindGeneric) {
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", badge, title, c.Text))
		for _, s := range c.Suggestions {
			lines = append(lines, "  • "+s)
		}
	} else {
		text := c.Text
		if c.Title != "" {
			text = c.Title + ": " + text
		}
		lines = append(lines, text)
	}
	return lines
}

func (c ErrorCell) LineCount() int { return len(c.RawLines()) }

// ThinkingCell represents an in-flight thought state in the transcript before
// any tokens stream from the model.
type ThinkingCell struct {
	Spinner string
}

func (ThinkingCell) Kind() HistoryCellKind { return HistoryCellAssistant }
func (c ThinkingCell) Render() []string    { return c.RenderWidth(defaultHistoryWidth) }
func (c ThinkingCell) RenderWidth(_ int) []string {
	indicator := "…"
	if c.Spinner != "" {
		indicator = c.Spinner
	}
	return []string{tuistyle.AssistantStyle.Render(indicator + " Thinking…")}
}
func (ThinkingCell) RawLines() []string { return []string{"Thinking…"} }
func (ThinkingCell) LineCount() int     { return 1 }
