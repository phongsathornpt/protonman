package style

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestBrandLockupUsesCompactLogoAtBreakpoint(t *testing.T) {
	for _, width := range []int{MinCompactBrandWidth, 40, 80} {
		got := ansi.Strip(BrandLockup(width))
		lines := strings.Split(got, "\n")
		if len(lines) != len(compactLogoLines) {
			t.Fatalf("width %d lines = %d, want %d: %q", width, len(lines), len(compactLogoLines), got)
		}
		for i, want := range compactLogoLines {
			if !strings.HasPrefix(lines[i], want) {
				t.Fatalf("width %d line %d = %q, want prefix %q", width, i, lines[i], want)
			}
		}
		if !strings.Contains(lines[0], ProductName) {
			t.Fatalf("width %d missing product name: %q", width, got)
		}
		// ProductName must start at CompactLogoTextColumn (column 11)
		idx := strings.Index(lines[0], ProductName)
		if idx != CompactLogoTextColumn {
			t.Fatalf("width %d ProductName at column %d, want %d: %q", width, idx, CompactLogoTextColumn, lines[0])
		}
	}
}

func TestCompactLogoGeometry(t *testing.T) {
	if got := CompactLogoWidth(); got != 7 {
		t.Fatalf("CompactLogoWidth() = %d, want 7", got)
	}
	if CompactLogoTextColumn != CompactLogoWidth()+4 {
		t.Fatalf("CompactLogoTextColumn = %d, want CompactLogoWidth() + 4 = %d", CompactLogoTextColumn, CompactLogoWidth()+4)
	}
}

func TestCompactBrand(t *testing.T) {
	if got := CompactBrand(0); got != "" {
		t.Fatalf("CompactBrand(0) = %q, want empty", got)
	}
	if got := ansi.Strip(CompactBrand(4)); got != "prot" {
		t.Fatalf("CompactBrand(4) = %q, want prot", got)
	}
	if got := ansi.Strip(CompactBrand(10)); got != ProductName {
		t.Fatalf("CompactBrand(10) = %q, want %q", got, ProductName)
	}
	if got := ansi.Strip(CompactBrand(11)); got != GlyphBrand+" "+ProductName {
		t.Fatalf("CompactBrand(11) = %q, want %q", got, GlyphBrand+" "+ProductName)
	}
	if got := ansi.Strip(CompactBrand(80)); got != GlyphBrand+" "+ProductName {
		t.Fatalf("CompactBrand(80) = %q, want %q", got, GlyphBrand+" "+ProductName)
	}
}

func TestBrandLockupFallsBackBelowBreakpoint(t *testing.T) {
	got := ansi.Strip(BrandLockup(MinCompactBrandWidth - 1))
	if strings.Contains(got, "/__\\") {
		t.Fatalf("fallback unexpectedly rendered ASCII logo: %q", got)
	}
	if !strings.Contains(got, ProductName) {
		t.Fatalf("fallback missing product name: %q", got)
	}
}

func TestBrandLockupNeverExceedsRequestedWidth(t *testing.T) {
	for _, width := range []int{0, 1, 4, 8, 9, 10, 11, 12, 23, 24, 32, 80} {
		got := BrandLockup(width)
		for _, line := range strings.Split(got, "\n") {
			if visual := ansi.StringWidth(line); visual > width {
				t.Fatalf("width %d rendered line width %d: %q", width, visual, ansi.Strip(line))
			}
		}
		if gotWidth := BrandLockupWidth(width); gotWidth > width {
			t.Fatalf("width %d reported lockup width %d", width, gotWidth)
		}
	}
}

func TestBrandLockupUltraNarrowTruncatesName(t *testing.T) {
	got := ansi.Strip(BrandLockup(4))
	if got != "prot" {
		t.Fatalf("ultra-narrow brand = %q, want %q", got, "prot")
	}
	if got := BrandLockup(0); got != "" {
		t.Fatalf("zero-width brand = %q, want empty", got)
	}
}
