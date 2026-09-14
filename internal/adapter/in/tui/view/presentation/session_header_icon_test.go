package presentation

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuiicon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/icon"
)

func TestCompactSessionHeaderUsesProvidedBrandIcon(t *testing.T) {
	got := ansi.Strip(RenderSessionHeader(SessionHeaderModel{
		Width:   25,
		Model:   "qwen3.8-27b",
		Compact: true,
		Icons:   tuiicon.Nerd,
	}))
	if !strings.Contains(got, tuiicon.Nerd.Brand) {
		t.Fatalf("compact header %q does not contain Nerd brand %q", got, tuiicon.Nerd.Brand)
	}
}

func TestSessionHeaderWithoutIconProfileKeepsUnicodeCompatibility(t *testing.T) {
	got := ansi.Strip(RenderSessionHeader(SessionHeaderModel{
		Width:   25,
		Compact: true,
	}))
	if !strings.Contains(got, tuiicon.Unicode.Brand) {
		t.Fatalf("compatibility header %q does not contain Unicode brand %q", got, tuiicon.Unicode.Brand)
	}
}
