package strutil

import "strings"

// SanitizeText strips unprintable ASCII control characters from text, preserving
// valid UTF-8 characters, spaces, newlines, and tabs.
func SanitizeText(text string) string {
	if text == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(text))
	for _, r := range text {
		if r == '\n' || r == '\t' || r == '\r' || (r >= 0x20 && r != 0x7f) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
