package style

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const MinASCIIBrandWidth = 48

var asciiBrandLines = [...]string{
	"█▀█ █▀▄ █▀█ ▀█▀ █▀█ █▄ █ █▀▄▀█ ▄▀█ █▄ █",
	"█▀▀ █▀▄ █▄█  █  █▄█ █ ▀█ █ ▀ █ █▀█ █ ▀█",
}

// BrandLockup renders a compact two-line terminal wordmark, with a narrow
// fallback that cannot wrap on cramped terminals.
func BrandLockup(width int) string {
	if width < MinASCIIBrandWidth {
		if width >= ansi.StringWidth(GlyphBrand+" protonman") {
			return BrandMarkStyle.Render(GlyphBrand) + " " + BrandStyle.Render("protonman")
		}
		return BrandStyle.Render("protonman")
	}

	first := BrandMarkStyle.Render(GlyphBrand) + "  " + BrandStyle.Render(asciiBrandLines[0])
	second := "   " + BrandStyle.Render(asciiBrandLines[1])
	return first + "\n" + second
}

func BrandLockupWidth(width int) int {
	maxWidth := 0
	for _, line := range strings.Split(BrandLockup(width), "\n") {
		maxWidth = max(maxWidth, ansi.StringWidth(line))
	}
	return maxWidth
}
