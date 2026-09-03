package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func wrapWords(text string, width int) string {
	if width < 8 || ansi.StringWidth(text) <= width {
		return text
	}
	words := strings.Fields(text)
	lines := make([]string, 0)
	var current string
	for _, word := range words {
		if ansi.StringWidth(word) > width {
			word = ansi.Truncate(word, width, "")
		}
		if current == "" {
			current = word
			continue
		}
		if ansi.StringWidth(current)+1+ansi.StringWidth(word) <= width {
			current += " " + word
			continue
		}
		lines = append(lines, current)
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
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
	var builder strings.Builder
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
