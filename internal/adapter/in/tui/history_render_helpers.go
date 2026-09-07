package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func rawTextLines(text string) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = sanitizeBubbleText(lines[i])
	}
	return lines
}

func renderStyledLines(text string, style func(string) string) []string {
	lines := rawTextLines(text)
	for i := range lines {
		lines[i] = style(lines[i])
	}
	return lines
}

func safeWrappedLines(text string, width int) []string {
	text = strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if text == "" {
		return nil
	}
	lines := make([]string, 0, strings.Count(text, "\n")+1)
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, wrapLines(sanitizeBubbleText(line), width)...)
	}
	return lines
}

func wrapStyledLines(styledText string, width int) []string {
	styledText = strings.TrimRight(strings.ReplaceAll(styledText, "\r\n", "\n"), "\n")
	if styledText == "" {
		return nil
	}
	lines := make([]string, 0, strings.Count(styledText, "\n")+1)
	for _, line := range strings.Split(styledText, "\n") {
		lines = append(lines, wrapLines(line, width)...)
	}
	return lines
}

func styledWrappedLines(text string, width int, style lipgloss.Style) []string {
	lines := safeWrappedLines(text, width)
	for index := range lines {
		lines[index] = style.Render(lines[index])
	}
	return lines
}
