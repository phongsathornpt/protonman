package presentation

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderSessionHeaderFull(t *testing.T) {
	got := ansi.Strip(RenderSessionHeader(SessionHeaderModel{Width: 80, Model: "qwen3.8-27b", LowConcurrency: true, GoalActive: true, Branch: "feat/tui-brand"}))
	want := "   /\\      protonMAN\n" +
		"  /__\\     qwen3.8-27b · low · goal active\n" +
		" <____>\n" +
		" /|__|\\    feat/tui-brand"
	if got != want {
		t.Fatalf("session header mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestRenderSessionHeaderResponsiveWidth(t *testing.T) {
	for _, width := range []int{8, 20, 24, 39, 40, 60, 80, 120} {
		got := RenderSessionHeader(SessionHeaderModel{Width: width, Model: "provider/a-very-long-model", LowConcurrency: true, GoalActive: true, Branch: "feat/tui-brand"})
		for _, line := range strings.Split(got, "\n") {
			if visual := ansi.StringWidth(line); visual > width {
				t.Fatalf("width=%d line width=%d: %q", width, visual, ansi.Strip(line))
			}
		}
	}
}

func TestRenderSessionHeaderMinimal(t *testing.T) {
	gotWide := ansi.Strip(RenderSessionHeader(SessionHeaderModel{Width: 80, Minimal: true}))
	if gotWide != "◆ protonMAN" {
		t.Fatalf("wide minimal header = %q, want '◆ protonMAN'", gotWide)
	}

	gotNarrow := ansi.Strip(RenderSessionHeader(SessionHeaderModel{Width: 5, Minimal: true}))
	if gotNarrow != "proto" {
		t.Fatalf("narrow minimal header = %q, want 'proto'", gotNarrow)
	}

	if gotZero := RenderSessionHeader(SessionHeaderModel{Width: 0, Minimal: true}); gotZero != "" {
		t.Fatalf("zero-width minimal header = %q, want empty", gotZero)
	}
}

func TestRenderSessionHeaderCompactUsesGlyphBrandAndEllipsis(t *testing.T) {
	got := ansi.Strip(RenderSessionHeader(SessionHeaderModel{
		Width:   25,
		Compact: true,
		Model:   "provider/a-very-long-model-name-exceeding-width",
	}))
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("compact lines = %d, want 2: %q", len(lines), got)
	}
	if lines[0] != "◆ protonMAN" {
		t.Fatalf("compact brand = %q, want '◆ protonMAN'", lines[0])
	}
	if !strings.HasSuffix(lines[1], "…") {
		t.Fatalf("compact meta missing ellipsis: %q", lines[1])
	}
}

func TestRenderSessionHeaderFullTruncatesWithEllipsis(t *testing.T) {
	got := ansi.Strip(RenderSessionHeader(SessionHeaderModel{
		Width:  42,
		Model:  "provider/a-very-long-model-that-must-truncate",
		Branch: "feature/super-long-branch-name-that-truncates",
	}))
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want 4: %q", len(lines), got)
	}
	if !strings.HasSuffix(lines[1], "…") {
		t.Fatalf("full header meta line 1 missing ellipsis: %q", lines[1])
	}
	if !strings.HasSuffix(lines[3], "…") {
		t.Fatalf("full header context line 3 missing ellipsis: %q", lines[3])
	}
}
