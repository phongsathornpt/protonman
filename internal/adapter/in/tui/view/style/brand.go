package style

import "github.com/charmbracelet/x/ansi"

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

// CompactBrandWithIcons renders Protonman's single-line compact brand identity
// with the supplied terminal capability profile. The full four-line ASCII logo
// remains unchanged because it does not depend on a patched font.
func CompactBrandWithIcons(width int, icons IconSet) string {
	if width <= 0 {
		return ""
	}
	icons = OrUnicodeIcons(icons)
	minGlyphWidth := ansi.StringWidth(icons.Brand + " " + ProductName)
	if width >= minGlyphWidth {
		return BrandMarkStyle.Render(icons.Brand) + " " + BrandStyle.Render(ProductName)
	}
	return BrandStyle.Render(ansi.Truncate(ProductName, width, ""))
}
