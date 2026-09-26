//go:build desktop || desktop_gio

package gioui

import (
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

type theme struct {
	material *material.Theme

	surface                 color.NRGBA
	surfaceDim              color.NRGBA
	surfaceBright           color.NRGBA
	surfaceContainerLowest  color.NRGBA
	surfaceContainerLow     color.NRGBA
	surfaceContainer        color.NRGBA
	surfaceContainerHigh    color.NRGBA
	surfaceContainerHighest color.NRGBA
	onSurface               color.NRGBA
	onSurfaceVariant        color.NRGBA
	outline                 color.NRGBA
	outlineVariant          color.NRGBA
	primary                 color.NRGBA
	onPrimary               color.NRGBA
	primaryContainer        color.NRGBA
	onPrimaryContainer      color.NRGBA
	inversePrimary          color.NRGBA
	secondary               color.NRGBA
	onSecondary             color.NRGBA
	secondaryContainer      color.NRGBA
	onSecondaryContainer    color.NRGBA
	tertiary                color.NRGBA
	onTertiary              color.NRGBA
	tertiaryContainer       color.NRGBA
	onTertiaryContainer     color.NRGBA
	errorContainer          color.NRGBA
	onErrorContainer        color.NRGBA
	successContainer        color.NRGBA
	onSuccessContainer      color.NRGBA
	warningContainer        color.NRGBA
	onWarningContainer      color.NRGBA

	// Subagent profile accents (Dota-style STR, AGI, INT)
	strengthContainer       color.NRGBA
	onStrengthContainer     color.NRGBA
	agilityContainer        color.NRGBA
	onAgilityContainer      color.NRGBA
	intelligenceContainer   color.NRGBA
	onIntelligenceContainer color.NRGBA
}

func newTheme(mode string) *theme {
	dark := strings.EqualFold(strings.TrimSpace(mode), "dark")
	baseline := material.NewTheme()
	baseline.Shaper = text.NewShaper()
	baseline.Face = font.Typeface("Roboto, Arial, sans-serif")
	baseline.TextSize = 16
	baseline.FingerSize = 44

	instance := &theme{material: baseline}
	if dark {
		instance.surface = rgba(0x10141a)
		instance.surfaceDim = rgba(0x0a0d10)
		instance.surfaceBright = rgba(0x2a323e)
		instance.surfaceContainerLowest = rgba(0x0a0d10)
		instance.surfaceContainerLow = rgba(0x14181e)
		instance.surfaceContainer = rgba(0x171d25)
		instance.surfaceContainerHigh = rgba(0x222a35)
		instance.surfaceContainerHighest = rgba(0x2c3542)
		instance.onSurface = rgba(0xe8eef6)
		instance.onSurfaceVariant = rgba(0xb2becc)
		instance.outline = rgba(0x64748b)
		instance.outlineVariant = rgba(0x3a4655)
		instance.primary = rgba(0x9db7ff)
		instance.onPrimary = rgba(0x182f68)
		instance.primaryContainer = rgba(0x233b78)
		instance.onPrimaryContainer = rgba(0xdce6ff)
		instance.inversePrimary = rgba(0x315be8)
		instance.secondary = rgba(0x94a3b8)
		instance.onSecondary = rgba(0x0f172a)
		instance.secondaryContainer = rgba(0x242e3a)
		instance.onSecondaryContainer = rgba(0xd2dce8)
		instance.tertiary = rgba(0xd8b4fe)
		instance.onTertiary = rgba(0x3b0764)
		instance.tertiaryContainer = rgba(0x3b1c56)
		instance.onTertiaryContainer = rgba(0xf3e8ff)
		instance.errorContainer = rgba(0x472b30)
		instance.onErrorContainer = rgba(0xffdadd)
		instance.successContainer = rgba(0x20392a)
		instance.onSuccessContainer = rgba(0xb8f0ca)
		instance.warningContainer = rgba(0x403316)
		instance.onWarningContainer = rgba(0xffe3a3)

		// Subagents
		instance.strengthContainer = rgba(0x431b06)
		instance.onStrengthContainer = rgba(0xfdba74)
		instance.agilityContainer = rgba(0x0a332c)
		instance.onAgilityContainer = rgba(0x5eead4)
		instance.intelligenceContainer = rgba(0x2e1065)
		instance.onIntelligenceContainer = rgba(0xddd6fe)
	} else {
		instance.surface = rgba(0xf6f7f9)
		instance.surfaceDim = rgba(0xdce1e8)
		instance.surfaceBright = rgba(0xffffff)
		instance.surfaceContainerLowest = rgba(0xffffff)
		instance.surfaceContainerLow = rgba(0xf0f3f6)
		instance.surfaceContainer = rgba(0xffffff)
		instance.surfaceContainerHigh = rgba(0xe9edf2)
		instance.surfaceContainerHighest = rgba(0xe0e5eb)
		instance.onSurface = rgba(0x17202b)
		instance.onSurfaceVariant = rgba(0x536171)
		instance.outline = rgba(0x72787e)
		instance.outlineVariant = rgba(0xc7cfd9)
		instance.primary = rgba(0x315be8)
		instance.onPrimary = rgba(0xffffff)
		instance.primaryContainer = rgba(0xe2e9ff)
		instance.onPrimaryContainer = rgba(0x183a9c)
		instance.inversePrimary = rgba(0x9db7ff)
		instance.secondary = rgba(0x4b5e78)
		instance.onSecondary = rgba(0xffffff)
		instance.secondaryContainer = rgba(0xe7ebf1)
		instance.onSecondaryContainer = rgba(0x2d3a4a)
		instance.tertiary = rgba(0x6b4f82)
		instance.onTertiary = rgba(0xffffff)
		instance.tertiaryContainer = rgba(0xf3e8ff)
		instance.onTertiaryContainer = rgba(0x4c1d95)
		instance.errorContainer = rgba(0xfde9e9)
		instance.onErrorContainer = rgba(0x8c2725)
		instance.successContainer = rgba(0xe4f5eb)
		instance.onSuccessContainer = rgba(0x1b6538)
		instance.warningContainer = rgba(0xfff1cc)
		instance.onWarningContainer = rgba(0x604600)

		// Subagents
		instance.strengthContainer = rgba(0xffedd5)
		instance.onStrengthContainer = rgba(0x7c2d12)
		instance.agilityContainer = rgba(0xccfbf1)
		instance.onAgilityContainer = rgba(0x115e59)
		instance.intelligenceContainer = rgba(0xede9fe)
		instance.onIntelligenceContainer = rgba(0x4c1d95)
	}
	baseline.Palette = material.Palette{
		Bg:         instance.surface,
		Fg:         instance.onSurface,
		ContrastBg: instance.primary,
		ContrastFg: instance.onPrimary,
	}
	return instance
}

func (t *theme) textFont(weight font.Weight) font.Font {
	return font.Font{Typeface: t.material.Face, Weight: weight}
}

func rgba(value uint32) color.NRGBA {
	return color.NRGBA{
		A: 0xff,
		R: uint8(value >> 16),
		G: uint8(value >> 8),
		B: uint8(value),
	}
}

const (
	shapeNone       unit.Dp = 0
	shapeExtraSmall unit.Dp = 4
	shapeSmall      unit.Dp = 8
	shapeMedium     unit.Dp = 12
	shapeLarge      unit.Dp = 16
	shapeExtraLarge unit.Dp = 24
	shapeFull       unit.Dp = 9999

	textDisplaySmall   unit.Sp = 30
	textHeadlineMedium unit.Sp = 24
	textHeadlineSmall  unit.Sp = 20
	textTitleLarge     unit.Sp = 18
	textTitleMedium    unit.Sp = 16
	textTitleSmall     unit.Sp = 14
	textBodyLarge      unit.Sp = 16
	textBodyMedium     unit.Sp = 14
	textBodySmall      unit.Sp = 12
	textLabelLarge     unit.Sp = 14
	textLabelMedium    unit.Sp = 12
	textLabelSmall     unit.Sp = 11
)
