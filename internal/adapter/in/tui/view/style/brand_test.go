package style

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCompactBrandWithIcons(t *testing.T) {
	if got := CompactBrandWithIcons(0, UnicodeIcons); got != "" {
		t.Fatalf("CompactBrandWithIcons(0) = %q, want empty", got)
	}
	if got := ansi.Strip(CompactBrandWithIcons(4, UnicodeIcons)); got != "prot" {
		t.Fatalf("CompactBrandWithIcons(4) = %q, want prot", got)
	}
	if got := ansi.Strip(CompactBrandWithIcons(10, UnicodeIcons)); got != ProductName {
		t.Fatalf("CompactBrandWithIcons(10) = %q, want %q", got, ProductName)
	}
	if got := ansi.Strip(CompactBrandWithIcons(11, UnicodeIcons)); got != GlyphBrand+" "+ProductName {
		t.Fatalf("CompactBrandWithIcons(11) = %q, want %q", got, GlyphBrand+" "+ProductName)
	}
	if got := ansi.Strip(CompactBrandWithIcons(80, UnicodeIcons)); got != GlyphBrand+" "+ProductName {
		t.Fatalf("CompactBrandWithIcons(80) = %q, want %q", got, GlyphBrand+" "+ProductName)
	}
	if got := ansi.Strip(CompactBrandWithIcons(MinCompactBrandWidth-1, UnicodeIcons)); !strings.Contains(got, ProductName) {
		t.Fatalf("fallback missing product name: %q", got)
	}
}

func TestCompactBrandWithIconsNeverExceedsRequestedWidth(t *testing.T) {
	for _, width := range []int{0, 1, 4, 8, 9, 10, 11, 12, 23, 24, 32, 80} {
		rendered := CompactBrandWithIcons(width, UnicodeIcons)
		for _, line := range strings.Split(rendered, "\n") {
			if visual := ansi.StringWidth(line); visual > width {
				t.Fatalf("width %d rendered line width %d: %q", width, visual, ansi.Strip(line))
			}
		}
	}
}
