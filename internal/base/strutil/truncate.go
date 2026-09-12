package strutil

import (
	"strings"
	"unicode/utf8"
)

// TruncateBytes truncates value to at most maxBytes, ensuring the result is valid UTF-8.
func TruncateBytes(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

// TruncateBytesWithMarker truncates value to at most maxBytes (including marker).
// Trailing newlines before marker are trimmed and a newline separates content and marker.
func TruncateBytesWithMarker(value string, maxBytes int, marker string) string {
	value = strings.ToValidUTF8(value, "")
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	if len(marker) >= maxBytes {
		return marker[:maxBytes]
	}
	prefixLimit := maxBytes - len(marker) - 1
	if prefixLimit <= 0 {
		return marker
	}
	if len(value) > prefixLimit {
		value = value[:prefixLimit]
		for len(value) > 0 && !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	return strings.TrimRight(value, "\n") + "\n" + marker
}

// TruncateRunes truncates value to at most maxRunes unicode code points.
func TruncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for i := range value {
		if count == maxRunes {
			return value[:i]
		}
		count++
	}
	return value
}

// TruncateRunesWithEllipsis truncates value to at most maxRunes code points, appending "…" if truncated.
func TruncateRunesWithEllipsis(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runeCount := utf8.RuneCountInString(value)
	if runeCount <= maxRunes {
		return value
	}
	if maxRunes == 1 {
		return "…"
	}
	return TruncateRunes(value, maxRunes-1) + "…"
}
