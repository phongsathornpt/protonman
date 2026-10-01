//go:build desktop || desktop_gio

package uikit

import (
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/text"
	"gioui.org/widget/material"
)

// Theme is the resolved desktop palette plus the Gio material theme the widgets
// need. Colors is the one colour vocabulary: components and the shell both read
// these fields directly, so a token can never drift between a palette and the
// struct handed to a component.
type Theme struct {
	Material *material.Theme
	Colors   Colors
}

// NewTheme resolves the requested mode into a Theme. An unrecognised mode falls
// back to the macOS-family palette so a bad preference can never produce an
// unstyled frame.
func NewTheme(mode string) *Theme {
	norm := strings.ToLower(strings.TrimSpace(mode))
	isSlate := strings.HasPrefix(norm, "slate")
	dark := strings.Contains(norm, "dark")

	baseline := material.NewTheme()
	baseline.Shaper = text.NewShaper()
	baseline.Face = font.Typeface("SF Pro, -apple-system, BlinkMacSystemFont, 'Helvetica Neue', Inter, system-ui, sans-serif")
	baseline.TextSize = 16
	baseline.FingerSize = 44

	instance := &Theme{Material: baseline}
	if isSlate {
		if dark {
			applySlateDarkTheme(instance)
		} else {
			applySlateLightTheme(instance)
		}
	} else {
		if dark {
			applyMacOSDarkTheme(instance)
		} else {
			applyMacOSLightTheme(instance)
		}
	}
	baseline.Palette = material.Palette{
		Bg:         instance.Colors.Surface,
		Fg:         instance.Colors.OnSurface,
		ContrastBg: instance.Colors.Primary,
		ContrastFg: instance.Colors.OnPrimary,
	}
	return instance
}

// TextFont returns the UI face at the requested weight.
func (t *Theme) TextFont(weight font.Weight) font.Font {
	return font.Font{Typeface: t.Material.Face, Weight: weight}
}

// MonoFont returns the monospace face at the requested weight.
func (t *Theme) MonoFont(weight font.Weight) font.Font {
	return font.Font{Typeface: font.Typeface("monospace"), Weight: weight}
}

func rgba(value uint32) color.NRGBA {
	return color.NRGBA{
		A: 0xff,
		R: uint8(value >> 16),
		G: uint8(value >> 8),
		B: uint8(value),
	}
}

func applyMacOSDarkTheme(instance *Theme) {
	instance.Colors.Surface = rgba(0x1e1e20)
	instance.Colors.SurfaceDim = rgba(0x18181a)
	instance.Colors.SurfaceBright = rgba(0x2c2c2e)
	instance.Colors.SurfaceContainerLowest = rgba(0x18181a)
	instance.Colors.SurfaceContainerLow = rgba(0x141416)
	instance.Colors.SurfaceContainer = rgba(0x252527)
	instance.Colors.SurfaceContainerHigh = rgba(0x2c2c2e)
	instance.Colors.SurfaceContainerHighest = rgba(0x3a3a3c)
	instance.Colors.OnSurface = rgba(0xf5f5f7)
	instance.Colors.OnSurfaceVariant = rgba(0xa1a1a6)
	instance.Colors.Outline = rgba(0x48484a)
	instance.Colors.OutlineVariant = rgba(0x2c2c2e)
	instance.Colors.Primary = rgba(0x0a84ff)
	instance.Colors.OnPrimary = rgba(0x001228)
	instance.Colors.PrimaryContainer = rgba(0x142c4d)
	instance.Colors.OnPrimaryContainer = rgba(0xbce0fd)
	instance.Colors.InversePrimary = rgba(0x0071e3)
	instance.Colors.Secondary = rgba(0x98989d)
	instance.Colors.OnSecondary = rgba(0x000000)
	instance.Colors.SecondaryContainer = rgba(0x2c2c2e)
	instance.Colors.OnSecondaryContainer = rgba(0xe5e5ea)
	instance.Colors.Tertiary = rgba(0xbf5af2)
	instance.Colors.OnTertiary = rgba(0x250638)
	instance.Colors.TertiaryContainer = rgba(0x381e54)
	instance.Colors.OnTertiaryContainer = rgba(0xf3e8ff)
	instance.Colors.ErrorContainer = rgba(0x441b1d)
	instance.Colors.OnErrorContainer = rgba(0xffa198)
	instance.Colors.SuccessContainer = rgba(0x133820)
	instance.Colors.OnSuccessContainer = rgba(0x7ee787)
	instance.Colors.WarningContainer = rgba(0x3d2e05)
	instance.Colors.OnWarningContainer = rgba(0xf6e05e)

	// Subagents
	instance.Colors.Strength = rgba(0xff9f0a)
	instance.Colors.StrengthContainer = rgba(0x431b06)
	instance.Colors.OnStrengthContainer = rgba(0xfdba74)
	instance.Colors.Agility = rgba(0x64d2ff)
	instance.Colors.AgilityContainer = rgba(0x0a332c)
	instance.Colors.OnAgilityContainer = rgba(0x5eead4)
	instance.Colors.Intelligence = rgba(0xbf5af2)
	instance.Colors.IntelligenceContainer = rgba(0x2e1065)
	instance.Colors.OnIntelligenceContainer = rgba(0xddd6fe)

	// Diffs
	instance.Colors.DiffAdded = rgba(0x30d158)
	instance.Colors.DiffAddedContainer = rgba(0x133820)
	instance.Colors.OnDiffAddedContainer = rgba(0x7ee787)
	instance.Colors.DiffDeleted = rgba(0xff453a)
	instance.Colors.DiffDeletedContainer = rgba(0x441b1d)
	instance.Colors.OnDiffDeletedContainer = rgba(0xffa198)
}

func applyMacOSLightTheme(instance *Theme) {
	instance.Colors.Surface = rgba(0xffffff)
	instance.Colors.SurfaceDim = rgba(0xf2f2f7)
	instance.Colors.SurfaceBright = rgba(0xffffff)
	instance.Colors.SurfaceContainerLowest = rgba(0xffffff)
	instance.Colors.SurfaceContainerLow = rgba(0xe5e5ea)
	instance.Colors.SurfaceContainer = rgba(0xffffff)
	instance.Colors.SurfaceContainerHigh = rgba(0xe4e6ea)
	instance.Colors.SurfaceContainerHighest = rgba(0xdcdfe4)
	instance.Colors.OnSurface = rgba(0x1d1d1f)
	instance.Colors.OnSurfaceVariant = rgba(0x59595e)
	instance.Colors.Outline = rgba(0xc7c7cc)
	instance.Colors.OutlineVariant = rgba(0xe5e5ea)
	instance.Colors.Primary = rgba(0x0071e3)
	instance.Colors.OnPrimary = rgba(0xffffff)
	instance.Colors.PrimaryContainer = rgba(0xe5f1ff)
	instance.Colors.OnPrimaryContainer = rgba(0x0055b3)
	instance.Colors.InversePrimary = rgba(0x58a6ff)
	instance.Colors.Secondary = rgba(0x6e6e73)
	instance.Colors.OnSecondary = rgba(0xffffff)
	instance.Colors.SecondaryContainer = rgba(0xe8eaed)
	instance.Colors.OnSecondaryContainer = rgba(0x1d1d1f)
	instance.Colors.Tertiary = rgba(0xaf52de)
	instance.Colors.OnTertiary = rgba(0xffffff)
	instance.Colors.TertiaryContainer = rgba(0xf5eafd)
	instance.Colors.OnTertiaryContainer = rgba(0x611c91)
	instance.Colors.ErrorContainer = rgba(0xfeeceb)
	instance.Colors.OnErrorContainer = rgba(0xc01e17)
	instance.Colors.SuccessContainer = rgba(0xe8f8ed)
	instance.Colors.OnSuccessContainer = rgba(0x156d2c)
	instance.Colors.WarningContainer = rgba(0xfff4e0)
	instance.Colors.OnWarningContainer = rgba(0x854400)

	// Subagents
	instance.Colors.Strength = rgba(0xe65100)
	instance.Colors.StrengthContainer = rgba(0xfff0e0)
	instance.Colors.OnStrengthContainer = rgba(0x872b00)
	instance.Colors.Agility = rgba(0x007a70)
	instance.Colors.AgilityContainer = rgba(0xe0f7f4)
	instance.Colors.OnAgilityContainer = rgba(0x004d47)
	instance.Colors.Intelligence = rgba(0x7b1fa2)
	instance.Colors.IntelligenceContainer = rgba(0xf3e5f5)
	instance.Colors.OnIntelligenceContainer = rgba(0x4a148c)

	// Diffs
	instance.Colors.DiffAdded = rgba(0x1a7f37)
	instance.Colors.DiffAddedContainer = rgba(0xe8f8ed)
	instance.Colors.OnDiffAddedContainer = rgba(0x156d2c)
	instance.Colors.DiffDeleted = rgba(0xcf222e)
	instance.Colors.DiffDeletedContainer = rgba(0xfeeceb)
	instance.Colors.OnDiffDeletedContainer = rgba(0xc01e17)
}

func applySlateDarkTheme(instance *Theme) {
	instance.Colors.Surface = rgba(0x0d1117)
	instance.Colors.SurfaceDim = rgba(0x080b0f)
	instance.Colors.SurfaceBright = rgba(0x1c2128)
	instance.Colors.SurfaceContainerLowest = rgba(0x080b0f)
	instance.Colors.SurfaceContainerLow = rgba(0x13171f)
	instance.Colors.SurfaceContainer = rgba(0x161b22)
	instance.Colors.SurfaceContainerHigh = rgba(0x21262d)
	instance.Colors.SurfaceContainerHighest = rgba(0x30363d)
	instance.Colors.OnSurface = rgba(0xf0f4f9)
	instance.Colors.OnSurfaceVariant = rgba(0x9ca3af)
	instance.Colors.Outline = rgba(0x484f58)
	instance.Colors.OutlineVariant = rgba(0x21262d)
	instance.Colors.Primary = rgba(0x58a6ff)
	instance.Colors.OnPrimary = rgba(0x0a1628)
	instance.Colors.PrimaryContainer = rgba(0x172b4d)
	instance.Colors.OnPrimaryContainer = rgba(0xbfdbfe)
	instance.Colors.InversePrimary = rgba(0x2563eb)
	instance.Colors.Secondary = rgba(0x94a3b8)
	instance.Colors.OnSecondary = rgba(0x0f172a)
	instance.Colors.SecondaryContainer = rgba(0x21262d)
	instance.Colors.OnSecondaryContainer = rgba(0xe2e8f0)
	instance.Colors.Tertiary = rgba(0xd8b4fe)
	instance.Colors.OnTertiary = rgba(0x3b0764)
	instance.Colors.TertiaryContainer = rgba(0x381e54)
	instance.Colors.OnTertiaryContainer = rgba(0xf3e8ff)
	instance.Colors.ErrorContainer = rgba(0x441b1d)
	instance.Colors.OnErrorContainer = rgba(0xffa198)
	instance.Colors.SuccessContainer = rgba(0x133820)
	instance.Colors.OnSuccessContainer = rgba(0x7ee787)
	instance.Colors.WarningContainer = rgba(0x3d2e05)
	instance.Colors.OnWarningContainer = rgba(0xf6e05e)

	// Subagents
	instance.Colors.Strength = rgba(0xf97316)
	instance.Colors.StrengthContainer = rgba(0x431b06)
	instance.Colors.OnStrengthContainer = rgba(0xfdba74)
	instance.Colors.Agility = rgba(0x14b8a6)
	instance.Colors.AgilityContainer = rgba(0x0a332c)
	instance.Colors.OnAgilityContainer = rgba(0x5eead4)
	instance.Colors.Intelligence = rgba(0xa855f7)
	instance.Colors.IntelligenceContainer = rgba(0x2e1065)
	instance.Colors.OnIntelligenceContainer = rgba(0xddd6fe)

	// Diffs
	instance.Colors.DiffAdded = rgba(0x3fb950)
	instance.Colors.DiffAddedContainer = rgba(0x133820)
	instance.Colors.OnDiffAddedContainer = rgba(0x7ee787)
	instance.Colors.DiffDeleted = rgba(0xf85149)
	instance.Colors.DiffDeletedContainer = rgba(0x441b1d)
	instance.Colors.OnDiffDeletedContainer = rgba(0xffa198)
}

func applySlateLightTheme(instance *Theme) {
	instance.Colors.Surface = rgba(0xf6f8fa)
	instance.Colors.SurfaceDim = rgba(0xeaeef2)
	instance.Colors.SurfaceBright = rgba(0xffffff)
	instance.Colors.SurfaceContainerLowest = rgba(0xffffff)
	instance.Colors.SurfaceContainerLow = rgba(0xf3f5f8)
	instance.Colors.SurfaceContainer = rgba(0xffffff)
	instance.Colors.SurfaceContainerHigh = rgba(0xebeff4)
	instance.Colors.SurfaceContainerHighest = rgba(0xe1e6eb)
	instance.Colors.OnSurface = rgba(0x1f2328)
	instance.Colors.OnSurfaceVariant = rgba(0x57606a)
	instance.Colors.Outline = rgba(0x8c959f)
	instance.Colors.OutlineVariant = rgba(0xd0d7de)
	instance.Colors.Primary = rgba(0x0969da)
	instance.Colors.OnPrimary = rgba(0xffffff)
	instance.Colors.PrimaryContainer = rgba(0xddf4ff)
	instance.Colors.OnPrimaryContainer = rgba(0x0969da)
	instance.Colors.InversePrimary = rgba(0x58a6ff)
	instance.Colors.Secondary = rgba(0x57606a)
	instance.Colors.OnSecondary = rgba(0xffffff)
	instance.Colors.SecondaryContainer = rgba(0xebf0f4)
	instance.Colors.OnSecondaryContainer = rgba(0x24292f)
	instance.Colors.Tertiary = rgba(0x8250df)
	instance.Colors.OnTertiary = rgba(0xffffff)
	instance.Colors.TertiaryContainer = rgba(0xfbefff)
	instance.Colors.OnTertiaryContainer = rgba(0x6639ba)
	instance.Colors.ErrorContainer = rgba(0xffebe9)
	instance.Colors.OnErrorContainer = rgba(0xcf222e)
	instance.Colors.SuccessContainer = rgba(0xdafbe1)
	instance.Colors.OnSuccessContainer = rgba(0x1a7f37)
	instance.Colors.WarningContainer = rgba(0xfff8c5)
	instance.Colors.OnWarningContainer = rgba(0x7d4e00)

	// Subagents
	instance.Colors.Strength = rgba(0xc2410c)
	instance.Colors.StrengthContainer = rgba(0xffedd5)
	instance.Colors.OnStrengthContainer = rgba(0x7c2d12)
	instance.Colors.Agility = rgba(0x0f766e)
	instance.Colors.AgilityContainer = rgba(0xccfbf1)
	instance.Colors.OnAgilityContainer = rgba(0x115e59)
	instance.Colors.Intelligence = rgba(0x7e22ce)
	instance.Colors.IntelligenceContainer = rgba(0xede9fe)
	instance.Colors.OnIntelligenceContainer = rgba(0x4c1d95)

	// Diffs
	instance.Colors.DiffAdded = rgba(0x1a7f37)
	instance.Colors.DiffAddedContainer = rgba(0xdafbe1)
	instance.Colors.OnDiffAddedContainer = rgba(0x1a7f37)
	instance.Colors.DiffDeleted = rgba(0xcf222e)
	instance.Colors.DiffDeletedContainer = rgba(0xffebe9)
	instance.Colors.OnDiffDeletedContainer = rgba(0xcf222e)
}
