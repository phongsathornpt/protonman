package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const minASCIIBrandWidth = 24

var asciiBrandLines = [...]string{
	"█▀█ █▀▄ █▀█ ▀█▀ █▀█ █▄ █",
	"█▀▀ █▀▄ █▄█  █  █▄█ █ ▀█",
}

// brandLockup renders a compact two-line terminal wordmark, with a narrow
// fallback that cannot wrap on cramped terminals.
func brandLockup(width int) string {
	if width < minASCIIBrandWidth {
		if width >= ansi.StringWidth(glyphBrand+" proton") {
			return brandMarkStyle.Render(glyphBrand) + " " + brandStyle.Render("proton")
		}
		return brandStyle.Render("proton")
	}

	first := brandMarkStyle.Render(glyphBrand) + "  " + brandStyle.Render(asciiBrandLines[0])
	second := "   " + brandStyle.Render(asciiBrandLines[1])
	return first + "\n" + second
}

func brandLockupWidth(width int) int {
	maxWidth := 0
	for _, line := range strings.Split(brandLockup(width), "\n") {
		maxWidth = max(maxWidth, ansi.StringWidth(line))
	}
	return maxWidth
}
