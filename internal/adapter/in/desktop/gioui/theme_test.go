//go:build desktop || desktop_gio

package gioui

import (
	"image/color"
	"math"
	"testing"
)

func TestThemeTextContrast(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		t.Run(mode, func(t *testing.T) {
			theme := newTheme(mode)
			pairs := []struct {
				name       string
				foreground color.NRGBA
				background color.NRGBA
			}{
				{name: "surface", foreground: theme.onSurface, background: theme.surface},
				{name: "surface variant", foreground: theme.onSurfaceVariant, background: theme.surface},
				{name: "primary", foreground: theme.onPrimary, background: theme.primary},
				{name: "primary container", foreground: theme.onPrimaryContainer, background: theme.primaryContainer},
				{name: "secondary container", foreground: theme.onSecondaryContainer, background: theme.secondaryContainer},
				{name: "tertiary container", foreground: theme.onTertiaryContainer, background: theme.tertiaryContainer},
				{name: "disabled", foreground: theme.onSurfaceVariant, background: theme.surfaceContainerHigh},
				{name: "error container", foreground: theme.onErrorContainer, background: theme.errorContainer},
				{name: "success container", foreground: theme.onSuccessContainer, background: theme.successContainer},
				{name: "warning container", foreground: theme.onWarningContainer, background: theme.warningContainer},
				{name: "strength container", foreground: theme.onStrengthContainer, background: theme.strengthContainer},
				{name: "agility container", foreground: theme.onAgilityContainer, background: theme.agilityContainer},
				{name: "intelligence container", foreground: theme.onIntelligenceContainer, background: theme.intelligenceContainer},
			}
			for _, pair := range pairs {
				t.Run(pair.name, func(t *testing.T) {
					if ratio := contrastRatio(pair.foreground, pair.background); ratio < 4.5 {
						t.Fatalf("contrast ratio %.2f is below 4.5", ratio)
					}
				})
			}
		})
	}
}

func contrastRatio(foreground, background color.NRGBA) float64 {
	light := math.Max(relativeLuminance(foreground), relativeLuminance(background))
	dark := math.Min(relativeLuminance(foreground), relativeLuminance(background))
	return (light + 0.05) / (dark + 0.05)
}

func relativeLuminance(value color.NRGBA) float64 {
	red := linearChannel(value.R)
	green := linearChannel(value.G)
	blue := linearChannel(value.B)
	return 0.2126*red + 0.7152*green + 0.0722*blue
}

func linearChannel(value uint8) float64 {
	channel := float64(value) / 255
	if channel <= 0.04045 {
		return channel / 12.92
	}
	return math.Pow((channel+0.055)/1.055, 2.4)
}
