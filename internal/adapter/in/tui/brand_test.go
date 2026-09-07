package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestBrandLockupResponsive(t *testing.T) {
	wide := brandLockup(80)
	lines := strings.Split(wide, "\n")
	if len(lines) != 2 {
		t.Fatalf("wide brand lines = %d, want 2: %q", len(lines), wide)
	}
	if !strings.Contains(ansi.Strip(lines[0]), glyphBrand) || !strings.Contains(ansi.Strip(wide), "█▀█") {
		t.Fatalf("wide brand missing mark/ascii wordmark: %q", wide)
	}
	if got := brandLockupWidth(80); got > 80 {
		t.Fatalf("wide brand width = %d, terminal width 80", got)
	}

	narrow := ansi.Strip(brandLockup(20))
	if strings.Contains(narrow, "█") || !strings.Contains(narrow, "proton") {
		t.Fatalf("narrow brand = %q, want compact proton fallback", narrow)
	}
	if got := brandLockupWidth(20); got > 20 {
		t.Fatalf("narrow brand width = %d, terminal width 20", got)
	}
}

func TestBrandMarkIsSingleCell(t *testing.T) {
	if got := ansi.StringWidth(glyphBrand); got != 1 {
		t.Fatalf("brand mark width = %d, want 1", got)
	}
}
