// Package textview contains terminal-safe text normalization and rendering primitives.
package textview

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func WrapWords(text string, width int) string {
	return strings.Join(WrapLines(text, width), "\n")
}

// wrapLines wraps text without dropping the tail of long tokens. The old
// implementation truncated a single long path/URL/command, which is unsafe in
// permission prompts and made tool output impossible to inspect completely.
func WrapLines(text string, width int) []string {
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

func Sanitize(text string) string {
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
