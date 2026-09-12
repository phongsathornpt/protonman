package strutil

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateBytes(t *testing.T) {
	tests := []struct {
		input    string
		maxBytes int
		expected string
	}{
		{"", 10, ""},
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 3, "hel"},
		{"hello", 0, ""},
		{"hello", -1, ""},
		// Multibyte: '世' is 3 bytes (0xE4, 0xB8, 0x96)
		{"hello世界", 8, "hello世"}, // 5 bytes + 3 bytes = 8 bytes exactly fits
		{"hello世界", 7, "hello"},  // 7 bytes cannot fit 3-byte '世', so drops back to "hello"
	}
	for _, tc := range tests {
		got := TruncateBytes(tc.input, tc.maxBytes)
		if got != tc.expected {
			t.Errorf("TruncateBytes(%q, %d) = %q; want %q", tc.input, tc.maxBytes, got, tc.expected)
		}
		if !utf8.ValidString(got) {
			t.Errorf("TruncateBytes(%q, %d) produced invalid UTF-8: %q", tc.input, tc.maxBytes, got)
		}
	}
}

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

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"normal text", "normal text"},
		{"hello\x00world\x07!", "helloworld!"},
		{"line1\nline2\twith\ttabs\r\n", "line1\nline2\twith\ttabs\r\n"},
		{"unicode: 世界 👋", "unicode: 世界 👋"},
		{"with delete \x7f char", "with delete  char"},
	}
	for _, tc := range tests {
		got := SanitizeText(tc.input)
		if got != tc.expected {
			t.Errorf("SanitizeText(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}
