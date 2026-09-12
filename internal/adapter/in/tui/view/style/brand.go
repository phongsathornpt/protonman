package style

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const (
	ProductName           = "protonMAN"
	MinCompactBrandWidth  = 24
	CompactLogoTextColumn = 11
)

var compactLogoLines = [...]string{
	`   /\`,
	`  /__\`,
	` <____>`,
	` /|__|\`,
}

// CompactLogoLines returns a copy of the canonical four-line terminal mark.
// Layout code owns the text column next to it so brand, model, and branch stay
// vertically aligned instead of inheriting per-line logo widths.
func CompactLogoLines() []string {
	return append([]string(nil), compactLogoLines[:]...)
}

// CompactLogoWidth returns the maximum visual cell width of the four-line mark.
func CompactLogoWidth() int {
	maxWidth := 0
	for _, line := range compactLogoLines {
		maxWidth = max(maxWidth, ansi.StringWidth(line))
	}
	return maxWidth
}

// CompactBrand renders Protonman's single-line compact brand identity,
// showing the single-cell mark glyph and product name when space permits,
// or falling back to the truncated product name on cramped widths.
func CompactBrand(width int) string {
	if width <= 0 {
		return ""
	}
	minGlyphWidth := ansi.StringWidth(GlyphBrand + " " + ProductName)
	if width >= minGlyphWidth {
		return BrandMarkStyle.Render(GlyphBrand) + " " + BrandStyle.Render(ProductName)
	}
	return BrandStyle.Render(ansi.Truncate(ProductName, width, ""))
}

// BrandLockup renders Protonman's compact character mark. The product name is
// aligned at CompactLogoTextColumn so text aligns consistently with the logo.
func BrandLockup(width int) string {
	if width <= 0 {
		return ""
	}
	if width < MinCompactBrandWidth {
		return CompactBrand(width)
	}

	lines := make([]string, len(compactLogoLines))
	for i, line := range compactLogoLines {
		lines[i] = BrandMarkStyle.Render(line)
	}
	gap := max(0, CompactLogoTextColumn-ansi.StringWidth(compactLogoLines[0]))
	available := max(1, width-CompactLogoTextColumn)
	lines[0] += strings.Repeat(" ", gap) + BrandStyle.Render(ansi.Truncate(ProductName, available, ""))
	return strings.Join(lines, "\n")
}

func BrandLockupWidth(width int) int {
	maxWidth := 0
	for _, line := range strings.Split(BrandLockup(width), "\n") {
		maxWidth = max(maxWidth, ansi.StringWidth(line))
	}
	return maxWidth
}
