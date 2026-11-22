package strutil

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateBytesWithMarker(t *testing.T) {
	tests := []struct {
		input    string
		maxBytes int
		marker   string
		expected string
	}{
		{"short", 10, "[truncated]", "short"},
		{"a very long string that needs truncation", 20, "[cut]", "a very long st\n[cut]"},
		{"test", 0, "[cut]", ""},
		{"test", -5, "[cut]", ""},
		{"test", 3, "[truncated]", "[tr"}, // maxBytes < len(marker)
		{"hello\n\nworld", 10, "[...]", "hell\n[...]"},
	}
	for _, tc := range tests {
		got := TruncateBytesWithMarker(tc.input, tc.maxBytes, tc.marker)
		if got != tc.expected {
			t.Errorf("TruncateBytesWithMarker(%q, %d, %q) = %q; want %q", tc.input, tc.maxBytes, tc.marker, got, tc.expected)
		}
		if !utf8.ValidString(got) {
			t.Errorf("TruncateBytesWithMarker produced invalid UTF-8: %q", got)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		input    string
		maxRunes int
		expected string
	}{
		{"", 5, ""},
		{"hello", 10, "hello"},
		{"hello", 3, "hel"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		{"世界你好", 2, "世界"},
		{"café", 3, "caf"},
	}
	for _, tc := range tests {
		got := TruncateRunes(tc.input, tc.maxRunes)
		if got != tc.expected {
			t.Errorf("TruncateRunes(%q, %d) = %q; want %q", tc.input, tc.maxRunes, got, tc.expected)
		}
	}
}

func TestTruncateRunesWithEllipsis(t *testing.T) {
	tests := []struct {
		input    string
		maxRunes int
		expected string
	}{
		{"", 5, ""},
		{"hello", 5, "hello"},
		{"hello world", 6, "hello…"},
		{"hello world", 1, "…"},
		{"hello world", 0, ""},
		{"世界你好吗", 4, "世界你…"},
	}
	for _, tc := range tests {
		got := TruncateRunesWithEllipsis(tc.input, tc.maxRunes)
		if got != tc.expected {
			t.Errorf("TruncateRunesWithEllipsis(%q, %d) = %q; want %q", tc.input, tc.maxRunes, got, tc.expected)
		}
	}
}
