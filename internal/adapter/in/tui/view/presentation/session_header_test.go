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
