package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/projectTHORN/proton/internal/core/tool"
)

// SystemCell renders low-priority status/history information.
type SystemCell struct{ Text string }

func (SystemCell) Kind() HistoryCellKind { return HistoryCellSystem }
func (c SystemCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c SystemCell) RenderWidth(width int) []string {
	return styledWrappedLines(c.Text, width, mutedStyle)
}
func (c SystemCell) RawLines() []string { return rawTextLines(c.Text) }
func (c SystemCell) LineCount() int     { return len(c.RawLines()) }

// ErrorCell renders a failed operation or classified OpenCode error.
type ErrorCell struct {
	Title       string
	Text        string
	Code        tool.ErrorCode
	ErrorKind   OpenCodeErrorKind
	Badge       string
	Suggestions []string
	RawDetails  string
	Retryable   bool
}

func (ErrorCell) Kind() HistoryCellKind { return HistoryCellError }
func (c ErrorCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }

func (c ErrorCell) RenderWidth(width int) []string {
	if width <= 0 {
		width = defaultBubbleWidth
	}

	// If this has structured error attributes (ErrorKind, Badge, or Suggestions),
	// render it as an OpenCode-style bordered error card.
	if c.Badge != "" || len(c.Suggestions) > 0 || (c.ErrorKind != "" && c.ErrorKind != ErrorKindGeneric) {
		return c.renderCard(width)
	}

	// Simple fallback rendering for legacy or simple tool errors
	text := c.Text
	if c.Title != "" {
		text = c.Title + ": " + text
	}
	return styledWrappedLines(glyphToolError+text, width, errorStyle)
}

func (c ErrorCell) renderCard(width int) []string {
	cardWidth := maxInt(24, width-2)
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
		Foreground(accentError).
		Render(fmt.Sprintf("%s[%s] %s", glyphToolError, badge, title))

	bodyLines := safeWrappedLines(c.Text, innerWidth)
	cardContent := []string{header}
	if len(bodyLines) > 0 {
		cardContent = append(cardContent, "")
		for _, bLine := range bodyLines {
			cardContent = append(cardContent, bodyStyle.Render(bLine))
		}
	}

	if len(c.Suggestions) > 0 {
		cardContent = append(cardContent, "")
		suggestHeader := lipgloss.NewStyle().Bold(true).Foreground(warningColor).Render("💡 Suggestions:")
		cardContent = append(cardContent, suggestHeader)
		for _, s := range c.Suggestions {
			wrappedS := safeWrappedLines("• "+s, innerWidth-2)
			for _, w := range wrappedS {
				cardContent = append(cardContent, mutedStyle.Render("  "+w))
			}
		}
	}

	joined := strings.Join(cardContent, "\n")
	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentError).
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
	if c.Badge != "" || len(c.Suggestions) > 0 || (c.ErrorKind != "" && c.ErrorKind != ErrorKindGeneric) {
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
func (c ThinkingCell) Render() []string    { return c.RenderWidth(defaultBubbleWidth) }
func (c ThinkingCell) RenderWidth(_ int) []string {
	indicator := "…"
	if c.Spinner != "" {
		indicator = c.Spinner
	}
	return []string{assistantStyle.Render(indicator + " Thinking…")}
}
func (ThinkingCell) RawLines() []string { return []string{"Thinking…"} }
func (ThinkingCell) LineCount() int     { return 1 }
