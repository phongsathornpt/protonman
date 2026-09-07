package mcp

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func TestDiagnosticFirstLineIsUnicodeAndANSISafe(t *testing.T) {
	input := "\x1b[31m" + strings.Repeat("ภาษาไทย🙂", 30) + "\x1b[0m\nsecret second line"
	got := diagnosticFirstLine(input, 24)
	if !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8: %q", got)
	}
	if strings.Contains(got, "\x1b[") || strings.Contains(got, "secret") {
		t.Fatalf("unsafe diagnostic = %q", got)
	}
	if width := ansi.StringWidth(got); width > 24 {
		t.Fatalf("width = %d, want <= 24: %q", width, got)
	}
}
