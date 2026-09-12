package style

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const (
	ProductName          = "protonMAN"
	MinCompactBrandWidth = 24
)

var compactLogoLines = [...]string{
	`   /\`,
	`  /__\`,
	` <____>`,
	` /|__|\`,
}

// BrandLockup renders Protonman's compact character mark. The product name is
// kept separate from the artwork so cramped terminals can fall back cleanly.
func BrandLockup(width int) string {
	if width <= 0 {
		return ""
	}
	if width < MinCompactBrandWidth {
		if width >= ansi.StringWidth(GlyphBrand+" "+ProductName) {
			return BrandMarkStyle.Render(GlyphBrand) + " " + BrandStyle.Render(ProductName)
		}
		return BrandStyle.Render(ansi.Truncate(ProductName, width, ""))
	}

	lines := make([]string, len(compactLogoLines))
	for i, line := range compactLogoLines {
		lines[i] = BrandMarkStyle.Render(line)
	}
	lines[0] += "    " + BrandStyle.Render(ProductName)
	return strings.Join(lines, "\n")
}

func BrandLockupWidth(width int) int {
	maxWidth := 0
	for _, line := range strings.Split(BrandLockup(width), "\n") {
		maxWidth = max(maxWidth, ansi.StringWidth(line))
	}
	return maxWidth
}
