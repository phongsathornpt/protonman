package textview

import (
	"strings"
	"sync"
	"unicode"

	"charm.land/lipgloss/v2"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

var (
	markdownH1Style     = tuistyle.MarkdownHeadingStyle
	markdownH2Style     = tuistyle.MarkdownH2Style
	markdownH3Style     = tuistyle.MarkdownH3Style
	markdownItalicStyle = tuistyle.MarkdownItalicStyle
	markdownCodeStyle   = tuistyle.MarkdownCodeStyle
	markdownQuoteStyle  = tuistyle.MarkdownQuoteStyle
	markdownBulletStyle = tuistyle.MarkdownBulletStyle
	markdownBoldStyle   = tuistyle.MarkdownBoldStyle
	markdownCodeInline  SimpleANSIStyle
	markdownBoldInline  SimpleANSIStyle
	markdownLinkInline  SimpleANSIStyle
	markdownEmInline    SimpleANSIStyle
)

// proseWidth caps prose measure so long model output stays readable on wide
// terminals. Fenced code deliberately keeps the full available width because
// its alignment carries meaning.
func proseWidth(width int) int {
	if width > tuistyle.MeasureProse {
		return tuistyle.MeasureProse
	}
	return width
}

func markdownHeadingStyleForLevel(level int) lipgloss.Style {
	switch {
	case level <= 1:
		return markdownH1Style
	case level == 2:
		return markdownH2Style
	default:
		return markdownH3Style
	}
}

type SimpleANSIStyle struct {
	once     sync.Once
	prefix   string
	suffix   string
	fallback bool
}

func (s *SimpleANSIStyle) WriteTo(out *strings.Builder, style lipgloss.Style, text string) {
	s.once.Do(func() {
		const marker = "__proton_style_probe__"
		rendered := style.Render(marker)
		index := strings.Index(rendered, marker)
		if index < 0 {
			s.fallback = true
			return
		}
		s.prefix = rendered[:index]
		s.suffix = rendered[index+len(marker):]
	})
	if s.fallback {
		out.WriteString(style.Render(text))
		return
	}
	out.WriteString(s.prefix)
	out.WriteString(text)
	out.WriteString(s.suffix)
}

// renderMarkdownLines provides a deliberately small, terminal-safe Markdown
// renderer for transcript cells. It covers the constructs that make model
// output scannable (headings, lists, quotes, inline code, links, and fences)
// while keeping all untrusted text sanitized before styling.
func RenderMarkdownLines(markdown string, width int) []string {
	if width < 8 {
		width = 8
	}
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := make([]string, 0, strings.Count(markdown, "\n")+1)
	state := MarkdownState{}
	for _, raw := range strings.Split(markdown, "\n") {
		lines = append(lines, RenderMarkdownLine(raw, width, &state)...)
	}
	if state.inFence {
		lines = append(lines, markdownCodeStyle.Render("  └─ code (unterminated)"))
	}
	return TrimTrailingBlankLines(lines)
}

type MarkdownState struct {
	inFence bool
}

// InFence reports whether incremental markdown parsing is inside a fenced code block.
func (s MarkdownState) InFence() bool { return s.inFence }

func RenderMarkdownLine(raw string, width int, state *MarkdownState) []string {
	line := Sanitize(raw)
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
		if state.inFence {
			state.inFence = false
			return []string{markdownCodeStyle.Render("  └─ code")}
		}
		state.inFence = true
		fenceLabel := strings.TrimSpace(strings.TrimLeft(trimmed[3:], "`~"))
		label := "code"
		if fenceLabel != "" {
			label += " · " + fenceLabel
		}
		return []string{markdownCodeStyle.Render("  ┌─ " + label)}
	}
	if state.inFence {
		wrapped := WrapLines(line, max(1, width-4))
		out := make([]string, 0, len(wrapped))
		for _, part := range wrapped {
			out = append(out, markdownCodeStyle.Render("  │ "+part))
		}
		return out
	}
	if trimmed == "" {
		return []string{""}
	}
	if heading, level, ok := markdownHeading(trimmed); ok {
		return RenderMarkdownWrapped(heading, width, markdownHeadingStyleForLevel(level))
	}
	if quote, ok := markdownQuote(trimmed); ok {
		wrapped := WrapLines(quote, max(1, proseWidth(width)-4))
		out := make([]string, 0, len(wrapped))
		for _, part := range wrapped {
			out = append(out, markdownQuoteStyle.Render("  │ "+part))
		}
		return out
	}
	if marker, item, ok := markdownListItem(trimmed); ok {
		itemWidth := max(1, proseWidth(width)-lipgloss.Width(marker)-2)
		wrapped := WrapLines(item, itemWidth)
		out := make([]string, 0, len(wrapped))
		for index, part := range wrapped {
			prefix := "    "
			if index == 0 {
				prefix = markdownBulletStyle.Render("  "+marker) + " "
			}
			out = append(out, prefix+tuistyle.BodyStyle.Render(styleInlineMarkdown(part)))
		}
		return out
	}
	return RenderMarkdownBodyWrapped(line, width)
}

func RenderMarkdownBodyWrapped(text string, width int) []string {
	wrapped := WrapLines(text, proseWidth(width))
	for index := range wrapped {
		wrapped[index] = tuistyle.BodyStyle.Render(styleInlineMarkdown(wrapped[index]))
	}
	return wrapped
}

func RenderMarkdownWrapped(text string, width int, style lipgloss.Style) []string {
	wrapped := WrapLines(text, proseWidth(width))
	for index := range wrapped {
		wrapped[index] = style.Render(styleInlineMarkdown(wrapped[index]))
	}
	return wrapped
}

func markdownHeading(line string) (string, int, bool) {
	count := 0
	for count < len(line) && line[count] == '#' {
		count++
	}
	if count == 0 || count > 6 || count >= len(line) || line[count] != ' ' {
		return "", 0, false
	}
	return strings.TrimSpace(line[count:]), count, true
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
	out.Grow(len(text))
	for index := 0; index < len(text); {
		plainStart := index
		for index < len(text) && text[index] != '`' && text[index] != '*' && text[index] != '[' {
			index++
		}
		if index > plainStart {
			out.WriteString(text[plainStart:index])
			if index == len(text) {
				break
			}
		}

		switch {
		case text[index] == '`':
			if end := strings.IndexByte(text[index+1:], '`'); end >= 0 {
				end += index + 1
				markdownCodeInline.WriteTo(&out, markdownCodeStyle, text[index+1:end])
				index = end + 1
				continue
			}
		case strings.HasPrefix(text[index:], "**"):
			if end := strings.Index(text[index+2:], "**"); end >= 0 {
				end += index + 2
				markdownBoldInline.WriteTo(&out, markdownBoldStyle, text[index+2:end])
				index = end + 2
				continue
			}
		case text[index] == '*':
			if end := emphasisEnd(text, index); end > index+1 {
				markdownEmInline.WriteTo(&out, markdownItalicStyle, text[index+1:end])
				index = end + 1
				continue
			}
		case text[index] == '[':
			if close := strings.IndexByte(text[index+1:], ']'); close >= 0 {
				close += index + 1
				if close+1 < len(text) && text[close+1] == '(' {
					if end := strings.IndexByte(text[close+2:], ')'); end >= 0 {
						markdownLinkInline.WriteTo(&out, tuistyle.CommandStyle, text[index+1:close])
						index = close + 2 + end + 1
						continue
					}
				}
			}
		}

		out.WriteByte(text[index])
		index++
	}
	return out.String()
}

// emphasisEnd returns the closing delimiter index for a single-asterisk
// emphasis span, or -1 when the asterisk is not a valid opener. Code spans win
// over emphasis, and space-padded asterisks stay literal.
func emphasisEnd(text string, open int) int {
	if open+1 >= len(text) || text[open+1] == ' ' || text[open+1] == '*' {
		return -1
	}
	for index := open + 1; index < len(text); index++ {
		switch text[index] {
		case '`':
			return -1
		case '*':
			if text[index-1] == ' ' {
				return -1
			}
			return index
		}
	}
	return -1
}

func TrimTrailingBlankLines(lines []string) []string {
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
