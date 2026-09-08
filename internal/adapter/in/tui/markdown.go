package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/textview"
)

var (
	markdownCodeStyle = tuistyle.MarkdownCodeStyle
	markdownBoldStyle = tuistyle.MarkdownBoldStyle
)

type simpleANSIStyle struct{ inner textview.SimpleANSIStyle }

func (s *simpleANSIStyle) writeTo(out *strings.Builder, style lipgloss.Style, text string) {
	s.inner.WriteTo(out, style, text)
}

func renderMarkdownLines(markdown string, width int) []string {
	return textview.RenderMarkdownLines(markdown, width)
}

func renderMarkdownBodyWrapped(text string, width int) []string {
	return textview.RenderMarkdownBodyWrapped(text, width)
}

func renderMarkdownWrapped(text string, width int, style lipgloss.Style) []string {
	return textview.RenderMarkdownWrapped(text, width, style)
}

type markdownRenderState = textview.MarkdownState

func renderMarkdownLine(raw string, width int, state *markdownRenderState) []string {
	return textview.RenderMarkdownLine(raw, width, state)
}

func trimTrailingBlankLines(lines []string) []string {
	return textview.TrimTrailingBlankLines(lines)
}
