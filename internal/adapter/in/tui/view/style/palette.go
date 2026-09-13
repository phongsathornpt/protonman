package style

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette contains raw visual colors only. Presentation code should consume
// semantic tokens from semantic.go instead of reaching into this palette.
// That keeps component intent stable when the visual palette changes.
type Palette struct {
	Canvas       color.Color
	TextPrimary  color.Color
	TextSecondary color.Color
	TextMuted    color.Color
	Brand        color.Color
	BrandHover   color.Color
	BrandStrong  color.Color
	Success      color.Color
	Warning      color.Color
	Danger       color.Color
	Info         color.Color
	Border       color.Color
	BorderSubtle color.Color
}

// DefaultDarkPalette is the canonical Protonman terminal palette.
// The near-black canvas is informational: terminals own the actual background,
// so components should avoid painting full-screen backgrounds.
var DefaultDarkPalette = Palette{
	Canvas:        lipgloss.Color("#0d0d0d"),
	TextPrimary:   lipgloss.Color("#f5f5f6"),
	TextSecondary: lipgloss.Color("#bcc1cb"),
	TextMuted:     lipgloss.Color("#8b929f"),
	Brand:         lipgloss.Color("#f0983c"),
	BrandHover:    lipgloss.Color("#ffab5c"),
	BrandStrong:   lipgloss.Color("#ea580c"),
	Success:       lipgloss.Color("#22c55e"),
	Warning:       lipgloss.Color("#eab308"),
	Danger:        lipgloss.Color("#f87171"),
	Info:          lipgloss.Color("#60a5fa"),
	Border:        lipgloss.Color("#2a2a2f"),
	BorderSubtle:  lipgloss.Color("#1d1d21"),
}
