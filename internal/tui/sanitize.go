package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func wrapWords(text string, width int) string {
	return strings.Join(wrapLines(text, width), "\n")
}

// wrapLines wraps text without dropping the tail of long tokens. The old
// implementation truncated a single long path/URL/command, which is unsafe in
// permission prompts and made tool output impossible to inspect completely.
func wrapLines(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	if isSingleLinePrintableASCII(text) {
		return wrapASCIILine(text, width)
	}
	lines := make([]string, 0, strings.Count(text, "\n")+1)
	for _, source := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if source == "" {
			lines = append(lines, "")
			continue
		}
		remaining := source
		firstChunk := true
		for remaining != "" {
			if !firstChunk {
				remaining = strings.TrimLeft(remaining, " \t")
				if remaining == "" {
					break
				}
			}
			firstChunk = false
			if ansi.StringWidth(remaining) <= width {
				lines = append(lines, remaining)
				break
			}
			cut := ansi.Cut(remaining, 0, width)
			if cut == "" {
				// A zero-width escape sequence or grapheme should never make the
				// loop spin forever.
				cut = string([]rune(remaining)[:1])
			}
			breakAt := strings.LastIndexAny(cut, " \t")
			if breakAt > 0 {
				candidate := strings.TrimRight(cut[:breakAt], " \t")
				if candidate != "" {
					lines = append(lines, candidate)
					remaining = strings.TrimLeft(remaining[breakAt:], " \t")
					continue
				}
			}
			lines = append(lines, cut)
			remaining = remaining[len(cut):]
		}
	}
	return lines
}

func isSingleLinePrintableASCII(text string) bool {
	for index := 0; index < len(text); index++ {
		value := text[index]
		if value < 0x20 || value >= 0x7f {
			return false
		}
	}
	return true
}

func wrapASCIILine(text string, width int) []string {
	if text == "" {
		return []string{""}
	}
	if len(text) <= width {
		return []string{text}
	}
	lines := make([]string, 0, (len(text)+width-1)/width)
	remaining := text
	firstChunk := true
	for remaining != "" {
		if !firstChunk {
			remaining = strings.TrimLeft(remaining, " \t")
			if remaining == "" {
				break
			}
		}
		firstChunk = false
		if len(remaining) <= width {
			lines = append(lines, remaining)
			break
		}
		cut := remaining[:width]
		if breakAt := strings.LastIndexAny(cut, " \t"); breakAt > 0 {
			candidate := strings.TrimRight(cut[:breakAt], " \t")
			if candidate != "" {
				lines = append(lines, candidate)
				remaining = remaining[breakAt:]
				continue
			}
		}
		lines = append(lines, cut)
		remaining = remaining[width:]
	}
	return lines
}

func overlayCenter(background, overlay string, width, height int) string {
	if width <= 0 || height <= 0 {
		return overlay
	}
	bgLines := strings.Split(background, "\n")
	lines := make([]string, height)
	for i := range lines {
		line := ""
		if i < len(bgLines) {
			line = bgLines[i]
		}
		lines[i] = padVisual(line, width)
	}

	fgLines := strings.Split(strings.TrimRight(overlay, "\n"), "\n")
	fgHeight := len(fgLines)
	if fgHeight > height {
		fgLines = fgLines[:height]
		fgHeight = height
	}
	fgWidth := 0
	for _, line := range fgLines {
		if w := ansi.StringWidth(line); w > fgWidth {
			fgWidth = w
		}
	}
	if fgWidth > width {
		fgWidth = width
	}
	top := (height - fgHeight) / 2
	left := (width - fgWidth) / 2
	if top < 0 {
		top = 0
	}
	if left < 0 {
		left = 0
	}
	for i := 0; i < fgHeight; i++ {
		fg := ansi.Truncate(fgLines[i], fgWidth, "")
		if w := ansi.StringWidth(fg); w < fgWidth {
			fg += strings.Repeat(" ", fgWidth-w)
		}
		lines[top+i] = spliceVisual(lines[top+i], fg, left, width)
	}
	return strings.Join(lines, "\n")
}

func padVisual(line string, width int) string {
	visual := ansi.StringWidth(line)
	if visual == width {
		return line
	}
	if visual > width {
		return ansi.Truncate(line, width, "")
	}
	return line + strings.Repeat(" ", width-visual)
}

func spliceVisual(dst, src string, left, width int) string {
	if left < 0 {
		left = 0
	}
	srcWidth := ansi.StringWidth(src)
	if left+srcWidth > width {
		src = ansi.Truncate(src, maxInt(0, width-left), "")
		srcWidth = ansi.StringWidth(src)
	}
	prefix := ansi.Cut(dst, 0, left)
	suffix := ""
	if end := left + srcWidth; end < width {
		suffix = ansi.Cut(dst, end, width)
	}
	return padVisual(prefix+src+suffix, width)
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func sanitizeBubbleText(text string) string {
	clean := true
	nonASCII := false
	for index := 0; index < len(text); index++ {
		value := text[index]
		if value == '\x1b' || value < 0x20 || value == 0x7f {
			clean = false
			break
		}
		if value >= utf8.RuneSelf {
			nonASCII = true
		}
	}
	if clean && (!nonASCII || utf8.ValidString(text)) {
		return text
	}

	var builder strings.Builder
	if len(text) > 0 {
		builder.Grow(len(text))
	}
	for index := 0; index < len(text); {
		if text[index] == '\x1b' {
			index++
			if index < len(text) && text[index] == '[' {
				index++
				for index < len(text) {
					value := text[index]
					index++
					if value >= '@' && value <= '~' {
						break
					}
				}
			}
			continue
		}
		value, size := utf8.DecodeRuneInString(text[index:])
		index += size
		if value < 0x20 || value == 0x7f {
			builder.WriteByte(' ')
			continue
		}
		builder.WriteRune(value)
	}
	return builder.String()
}
