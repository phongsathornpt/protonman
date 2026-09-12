package history

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/diagnostic"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// SystemCell renders low-priority status/history information.
type SystemCell struct{ Text string }

func (SystemCell) Kind() HistoryCellKind { return HistoryCellSystem }
func (c SystemCell) RenderWidth(width int) []string {
	return styledWrappedLines(c.Text, width, tuistyle.MutedStyle)
}
func (c SystemCell) RawLines() []string { return rawTextLines(c.Text) }
func (c SystemCell) LineCount() int     { return len(c.RawLines()) }

// ErrorCell renders a failed operation or classified OpenCode error.
type ErrorCell struct {
	Title       string
	Target      string
	Text        string
	Code        tool.ErrorCode
	ErrorKind   diagnostic.Kind
	Badge       string
	Suggestions []string
	RawDetails  string
	Retryable   bool
}

func (ErrorCell) Kind() HistoryCellKind { return HistoryCellError }

func (c ErrorCell) RenderWidth(width int) []string {
	if width <= 0 {
		width = defaultHistoryWidth
	}

	if c.ErrorKind == diagnostic.KindToolFailed {
		return c.renderCompactToolFailure(width)
	}

	return c.renderCompactDiagnostic(width)
}

func (c ErrorCell) renderCompactDiagnostic(width int) []string {
	title := strings.TrimSpace(c.Title)
	if title == "" {
		title = "Error"
	}
	code := ""
	if c.ErrorKind != "" && c.ErrorKind != diagnostic.KindGeneric {
		code = diagnostic.UserCode(c.ErrorKind)
	}

	header := tuistyle.GlyphToolError + title
	if code != "" {
		header += tuistyle.GlyphSep + code
	}
	if c.ErrorKind == "" || c.ErrorKind == diagnostic.KindGeneric {
		if text := strings.TrimSpace(c.Text); text != "" {
			header += ": " + text
		}
	}
	return styledWrappedLines(header, width, tuistyle.ErrorStyle)
}

func (c ErrorCell) renderCompactToolFailure(width int) []string {
	badge := strings.TrimSpace(c.Badge)
	if badge == "" && c.Code != "" {
		badge = string(c.Code)
	}
	badge = strings.ReplaceAll(strings.ToLower(badge), "_", " ")
	title := strings.TrimSpace(c.Title)
	if title == "" {
		title = "tool"
	}

	header := tuistyle.ErrorStyle.Render(tuistyle.GlyphToolError + title)
	if badge != "" {
		header += tuistyle.MutedStyle.Render(tuistyle.GlyphSep + badge)
	}
	lines := wrapStyledLines(header, width)
	if target := strings.TrimSpace(c.Target); target != "" {
		lines = append(lines, indentedMutedLines(target, width, "  ", "  ")...)
	} else if text := strings.TrimSpace(c.Text); text != "" {
		lines = append(lines, indentedMutedLines(text, width, "  ", "  ")...)
	}
	for _, suggestion := range c.Suggestions {
		lines = append(lines, indentedMutedLines(suggestion, width, "  ↳ ", "    ")...)
	}
	return lines
}

func indentedMutedLines(text string, width int, firstPrefix, continuationPrefix string) []string {
	contentWidth := width - len([]rune(firstPrefix))
	if contentWidth < 1 {
		contentWidth = 1
	}
	wrapped := safeWrappedLines(strings.TrimSpace(text), contentWidth)
	for i := range wrapped {
		prefix := continuationPrefix
		if i == 0 {
			prefix = firstPrefix
		}
		wrapped[i] = tuistyle.MutedStyle.Render(prefix + wrapped[i])
	}
	return wrapped
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
		detail := c.Text
		if strings.TrimSpace(c.Target) != "" {
			detail = strings.TrimSpace(c.Target) + ": " + detail
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", badge, title, detail))
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
