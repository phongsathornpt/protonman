package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

var (
	markdownHeadingStyle = lipgloss.NewStyle().Bold(true).Foreground(accentAssistant)
	markdownCodeStyle    = lipgloss.NewStyle().Foreground(accentSystem)
	markdownQuoteStyle   = lipgloss.NewStyle().Foreground(accentTool)
	markdownBulletStyle  = lipgloss.NewStyle().Foreground(accentAssistant)
	markdownBoldStyle    = lipgloss.NewStyle().Bold(true)
)

// renderMarkdownLines provides a deliberately small, terminal-safe Markdown
// renderer for transcript cells. It covers the constructs that make model
// output scannable (headings, lists, quotes, inline code, links, and fences)
// while keeping all untrusted text sanitized before styling.
func renderMarkdownLines(markdown string, width int) []string {
	if width < 8 {
		width = 8
	}
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := make([]string, 0, strings.Count(markdown, "\n")+1)
	inFence := false
	fenceLabel := ""

	for _, raw := range strings.Split(markdown, "\n") {
		line := sanitizeBubbleText(raw)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if inFence {
				lines = append(lines, markdownCodeStyle.Render("  └─ code"))
				inFence = false
				fenceLabel = ""
				continue
			}
			inFence = true
			fenceLabel = strings.TrimSpace(strings.TrimLeft(trimmed[3:], "`~"))
			label := "code"
			if fenceLabel != "" {
				label += " · " + fenceLabel
			}
			lines = append(lines, markdownCodeStyle.Render("  ┌─ "+label))
			continue
		}
		if inFence {
			for _, wrapped := range wrapLines(line, maxInt(1, width-4)) {
				lines = append(lines, markdownCodeStyle.Render("  │ "+wrapped))
			}
			continue
		}
		if trimmed == "" {
			lines = append(lines, "")
			continue
		}

		if heading, ok := markdownHeading(trimmed); ok {
			lines = append(lines, renderMarkdownWrapped(heading, width, markdownHeadingStyle)...)
			continue
		}
		if quote, ok := markdownQuote(trimmed); ok {
			for _, wrapped := range wrapLines(quote, maxInt(1, width-4)) {
				lines = append(lines, markdownQuoteStyle.Render("  │ "+wrapped))
			}
			continue
		}
		if marker, item, ok := markdownListItem(trimmed); ok {
			itemWidth := maxInt(1, width-lipgloss.Width(marker)-2)
			wrapped := wrapLines(item, itemWidth)
			for index, part := range wrapped {
				prefix := "    "
				if index == 0 {
					prefix = markdownBulletStyle.Render("  "+marker) + " "
				}
				lines = append(lines, prefix+styleInlineMarkdown(part))
			}
			continue
		}
		lines = append(lines, renderMarkdownWrapped(line, width, bodyStyle)...)
	}

	if inFence {
		lines = append(lines, markdownCodeStyle.Render("  └─ code (unterminated)"))
	}
	return trimTrailingBlankLines(lines)
}

func renderMarkdownWrapped(text string, width int, style lipgloss.Style) []string {
	wrapped := wrapLines(text, width)
	for index := range wrapped {
		wrapped[index] = style.Render(styleInlineMarkdown(wrapped[index]))
	}
	return wrapped
}

func markdownHeading(line string) (string, bool) {
	count := 0
	for count < len(line) && line[count] == '#' {
		count++
	}
	if count == 0 || count > 6 || count >= len(line) || line[count] != ' ' {
		return "", false
	}
	return strings.TrimSpace(line[count:]), true
}

func markdownQuote(line string) (string, bool) {
	if !strings.HasPrefix(line, ">") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, ">")), true
}

func markdownListItem(line string) (marker, item string, ok bool) {
	if len(line) >= 2 && (line[0] == '-' || line[0] == '*' || line[0] == '+') && unicode.IsSpace(rune(line[1])) {
		return string(line[0]), strings.TrimSpace(line[2:]), true
	}
	for index := 0; index < len(line); index++ {
		if line[index] < '0' || line[index] > '9' {
			break
		}
		if index+1 < len(line) && line[index+1] == '.' && index+2 < len(line) && unicode.IsSpace(rune(line[index+2])) {
			return line[:index+2], strings.TrimSpace(line[index+2:]), true
		}
	}
	return "", "", false
}

func styleInlineMarkdown(text string) string {
	var out strings.Builder
	for index := 0; index < len(text); {
		switch {
		case text[index] == '`':
			if end := strings.IndexByte(text[index+1:], '`'); end >= 0 {
				end += index + 1
				out.WriteString(markdownCodeStyle.Render(text[index+1 : end]))
				index = end + 1
				continue
			}
		case strings.HasPrefix(text[index:], "**"):
			if end := strings.Index(text[index+2:], "**"); end >= 0 {
				end += index + 2
				out.WriteString(markdownBoldStyle.Render(text[index+2 : end]))
				index = end + 2
				continue
			}
		case text[index] == '[':
			if close := strings.IndexByte(text[index+1:], ']'); close >= 0 {
				close += index + 1
				if close+1 < len(text) && text[close+1] == '(' {
					if end := strings.IndexByte(text[close+2:], ')'); end >= 0 {
						out.WriteString(commandStyle.Render(text[index+1 : close]))
						index = close + 2 + end + 1
						continue
					}
				}
			}
		}
		runeValue, size := decodeRune(text[index:])
		out.WriteRune(runeValue)
		index += size
	}
	return out.String()
}

func decodeRune(text string) (rune, int) {
	if text == "" {
		return 0, 0
	}
	return utf8.DecodeRuneInString(text)
}

func trimTrailingBlankLines(lines []string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
