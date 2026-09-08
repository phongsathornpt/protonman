package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/textview"
)

func wrapWords(text string, width int) string   { return textview.WrapWords(text, width) }
func wrapLines(text string, width int) []string { return textview.WrapLines(text, width) }
func sanitizeBubbleText(text string) string     { return textview.Sanitize(text) }

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
