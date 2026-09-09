package textview

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTerminalCellWidthUnicode(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{"กำลัง", 3},
		{"กั", 1},
		{"你好", 4},
		{"👨‍💻", 2},
	} {
		if got := Width(tc.text); got != tc.want {
			t.Errorf("Width(%q)=%d want=%d", tc.text, got, tc.want)
		}
	}
}

func TestTruncateEllipsisPreservesGraphemeClusters(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width int
		want  string
	}{
		{"กัขค", 2, "กั…"},
		{"👨‍💻abc", 3, "👨‍💻…"},
		{"你好世界", 5, "你好…"},
	} {
		got := TruncateEllipsis(tc.text, tc.width)
		if got != tc.want {
			t.Errorf("TruncateEllipsis(%q,%d)=%q want=%q", tc.text, tc.width, got, tc.want)
		}
		if !utf8.ValidString(got) || Width(got) > tc.width {
			t.Errorf("invalid terminal truncation %q width=%d", got, Width(got))
		}
	}
}

func TestPadRightUsesTerminalCells(t *testing.T) {
	got := PadRight("กั", 4)
	if Width(got) != 4 || !strings.HasPrefix(got, "กั") {
		t.Fatalf("PadRight=%q width=%d", got, Width(got))
	}
}

func TestWrapLinesKeepsThaiGraphemeClustersIntact(t *testing.T) {
	got := WrapLines("กัขค", 1)
	want := []string{"กั", "ข", "ค"}
	if len(got) != len(want) {
		t.Fatalf("WrapLines=%q want=%q", got, want)
	}
	for i := range want {
		if got[i] != want[i] || !utf8.ValidString(got[i]) || Width(got[i]) > 1 {
			t.Fatalf("WrapLines[%d]=%q want=%q width=%d", i, got[i], want[i], Width(got[i]))
		}
	}
}

func TestWrapLinesPreservesANSISequencesAcrossNarrowWrap(t *testing.T) {
	styled := "\x1b[90m/workspace/project/internal/\x1b[m\x1b[1;36ma-very-long-file-name.go\x1b[m\x1b[90m …\x1b[m"
	lines := WrapLines(styled, 24)
	plain := ansi.Strip(strings.Join(lines, ""))
	if plain != "/workspace/project/internal/a-very-long-file-name.go …" {
		t.Fatalf("wrapped ANSI text corrupted visible content: %q", plain)
	}
	for _, line := range lines {
		if width := Width(line); width > 24 {
			t.Fatalf("wrapped ANSI line width=%d want <=24: %q", width, line)
		}
	}
}

func TestTruncateLeftEllipsisPreservesUsefulSuffix(t *testing.T) {
	got := TruncateLeftEllipsis("/workspace/project/file.go", 12)
	if got != "…ect/file.go" {
		t.Fatalf("left truncation = %q", got)
	}
	if Width(got) > 12 {
		t.Fatalf("left truncation width=%d", Width(got))
	}
}
