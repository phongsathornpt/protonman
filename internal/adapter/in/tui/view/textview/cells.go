package textview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Width returns the visible terminal-cell width of text, ignoring ANSI control sequences.
func Width(text string) int { return ansi.StringWidth(text) }

// Truncate clips text to at most width terminal cells without splitting a grapheme cluster.
func Truncate(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if Width(text) <= width {
		return text
	}
	return ansi.Truncate(text, width, "")
}

// TruncateEllipsis clips text to width terminal cells and appends an ellipsis when truncated.
func TruncateEllipsis(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if Width(text) <= width {
		return text
	}
	ellipsis := "…"
	ellipsisWidth := Width(ellipsis)
	if width <= ellipsisWidth {
		return Truncate(ellipsis, width)
	}
	return ansi.Truncate(text, width-ellipsisWidth, "") + ellipsis
}

// TruncateLeftEllipsis clips text from the left while preserving its most useful suffix.
func TruncateLeftEllipsis(text string, width int) string {
	if width <= 0 {
		return ""
	}
	total := Width(text)
	if total <= width {
		return text
	}
	ellipsis := "…"
	ellipsisWidth := Width(ellipsis)
	if width <= ellipsisWidth {
		return Truncate(ellipsis, width)
	}
	return ellipsis + ansi.Cut(text, total-(width-ellipsisWidth), total)
}

// PadRight pads text with spaces until it occupies width terminal cells.
func PadRight(text string, width int) string {
	if width <= 0 {
		return ""
	}
	visible := Width(text)
	if visible >= width {
		return text
	}
	return text + strings.Repeat(" ", width-visible)
}

// Fit truncates then right-pads text to exactly width terminal cells when possible.
func Fit(text string, width int) string {
	return PadRight(Truncate(text, width), width)
}
