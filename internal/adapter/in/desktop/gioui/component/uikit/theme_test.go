//go:build desktop || desktop_gio

package uikit

import (
	"image/color"
	"math"
	"testing"
)

func TestThemeTextContrast(t *testing.T) {
	for _, mode := range []string{"light", "dark", "macos-light", "macos-dark", "slate-light", "slate-dark"} {
		t.Run(mode, func(t *testing.T) {
			theme := NewTheme(mode)
			pairs := []struct {
				name       string
				foreground color.NRGBA
				background color.NRGBA
			}{
				{name: "surface", foreground: theme.Colors.OnSurface, background: theme.Colors.Surface},
				{name: "surface variant", foreground: theme.Colors.OnSurfaceVariant, background: theme.Colors.Surface},
				{name: "primary", foreground: theme.Colors.OnPrimary, background: theme.Colors.Primary},
				{name: "primary container", foreground: theme.Colors.OnPrimaryContainer, background: theme.Colors.PrimaryContainer},
				{name: "secondary container", foreground: theme.Colors.OnSecondaryContainer, background: theme.Colors.SecondaryContainer},
				{name: "tertiary container", foreground: theme.Colors.OnTertiaryContainer, background: theme.Colors.TertiaryContainer},
				{name: "disabled", foreground: theme.Colors.OnSurfaceVariant, background: theme.Colors.SurfaceContainerHigh},
				{name: "error container", foreground: theme.Colors.OnErrorContainer, background: theme.Colors.ErrorContainer},
				{name: "success container", foreground: theme.Colors.OnSuccessContainer, background: theme.Colors.SuccessContainer},
				{name: "warning container", foreground: theme.Colors.OnWarningContainer, background: theme.Colors.WarningContainer},
				{name: "strength container", foreground: theme.Colors.OnStrengthContainer, background: theme.Colors.StrengthContainer},
				{name: "agility container", foreground: theme.Colors.OnAgilityContainer, background: theme.Colors.AgilityContainer},
				{name: "intelligence container", foreground: theme.Colors.OnIntelligenceContainer, background: theme.Colors.IntelligenceContainer},
				{name: "diff added container", foreground: theme.Colors.OnDiffAddedContainer, background: theme.Colors.DiffAddedContainer},
				{name: "diff deleted container", foreground: theme.Colors.OnDiffDeletedContainer, background: theme.Colors.DiffDeletedContainer},
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
