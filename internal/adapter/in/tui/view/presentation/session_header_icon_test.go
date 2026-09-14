package presentation

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

func TestCompactSessionHeaderUsesProvidedBrandIcon(t *testing.T) {
	got := ansi.Strip(RenderSessionHeader(SessionHeaderModel{
		Width:   25,
		Model:   "qwen3.8-27b",
		Compact: true,
		Icons:   tuistyle.NerdIcons,
	}))
	if !strings.Contains(got, tuistyle.NerdIcons.Brand) {
		t.Fatalf("compact header %q does not contain Nerd brand %q", got, tuistyle.NerdIcons.Brand)
	}
}

func TestSessionHeaderZeroIconProfileKeepsUnicodeCompatibility(t *testing.T) {
	got := ansi.Strip(RenderSessionHeader(SessionHeaderModel{
		Width:   25,
		Compact: true,
	}))
	if !strings.Contains(got, tuistyle.UnicodeIcons.Brand) {
		t.Fatalf("compatibility header %q does not contain Unicode brand %q", got, tuistyle.UnicodeIcons.Brand)
	}
}
