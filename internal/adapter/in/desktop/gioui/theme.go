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
	strength                color.NRGBA
	strengthContainer       color.NRGBA
	onStrengthContainer     color.NRGBA
	agility                 color.NRGBA
	agilityContainer        color.NRGBA
	onAgilityContainer      color.NRGBA
	intelligence            color.NRGBA
	intelligenceContainer   color.NRGBA
	onIntelligenceContainer color.NRGBA

	// Diffs
	diffAdded              color.NRGBA
	diffAddedContainer     color.NRGBA
	onDiffAddedContainer   color.NRGBA
	diffDeleted            color.NRGBA
	diffDeletedContainer   color.NRGBA
	onDiffDeletedContainer color.NRGBA
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
		instance.surface = rgba(0x0d1117)
		instance.surfaceDim = rgba(0x080b0f)
		instance.surfaceBright = rgba(0x1c2128)
		instance.surfaceContainerLowest = rgba(0x080b0f)
		instance.surfaceContainerLow = rgba(0x13171f)
		instance.surfaceContainer = rgba(0x161b22)
		instance.surfaceContainerHigh = rgba(0x21262d)
		instance.surfaceContainerHighest = rgba(0x30363d)
		instance.onSurface = rgba(0xf0f4f9)
		instance.onSurfaceVariant = rgba(0x9ca3af)
		instance.outline = rgba(0x484f58)
		instance.outlineVariant = rgba(0x21262d)
		instance.primary = rgba(0x58a6ff)
		instance.onPrimary = rgba(0x0a1628)
		instance.primaryContainer = rgba(0x172b4d)
		instance.onPrimaryContainer = rgba(0xbfdbfe)
		instance.inversePrimary = rgba(0x2563eb)
		instance.secondary = rgba(0x94a3b8)
		instance.onSecondary = rgba(0x0f172a)
		instance.secondaryContainer = rgba(0x21262d)
		instance.onSecondaryContainer = rgba(0xe2e8f0)
		instance.tertiary = rgba(0xd8b4fe)
		instance.onTertiary = rgba(0x3b0764)
		instance.tertiaryContainer = rgba(0x381e54)
		instance.onTertiaryContainer = rgba(0xf3e8ff)
		instance.errorContainer = rgba(0x441b1d)
		instance.onErrorContainer = rgba(0xffa198)
		instance.successContainer = rgba(0x133820)
		instance.onSuccessContainer = rgba(0x7ee787)
		instance.warningContainer = rgba(0x3d2e05)
		instance.onWarningContainer = rgba(0xf6e05e)

		// Subagents
		instance.strength = rgba(0xf97316)
		instance.strengthContainer = rgba(0x431b06)
		instance.onStrengthContainer = rgba(0xfdba74)
		instance.agility = rgba(0x14b8a6)
		instance.agilityContainer = rgba(0x0a332c)
		instance.onAgilityContainer = rgba(0x5eead4)
		instance.intelligence = rgba(0xa855f7)
		instance.intelligenceContainer = rgba(0x2e1065)
		instance.onIntelligenceContainer = rgba(0xddd6fe)

		// Diffs
		instance.diffAdded = rgba(0x3fb950)
		instance.diffAddedContainer = rgba(0x133820)
		instance.onDiffAddedContainer = rgba(0x7ee787)
		instance.diffDeleted = rgba(0xf85149)
		instance.diffDeletedContainer = rgba(0x441b1d)
		instance.onDiffDeletedContainer = rgba(0xffa198)
	} else {
		instance.surface = rgba(0xf6f8fa)
		instance.surfaceDim = rgba(0xeaeef2)
		instance.surfaceBright = rgba(0xffffff)
		instance.surfaceContainerLowest = rgba(0xffffff)
		instance.surfaceContainerLow = rgba(0xf3f5f8)
		instance.surfaceContainer = rgba(0xffffff)
		instance.surfaceContainerHigh = rgba(0xebeff4)
		instance.surfaceContainerHighest = rgba(0xe1e6eb)
		instance.onSurface = rgba(0x1f2328)
		instance.onSurfaceVariant = rgba(0x57606a)
		instance.outline = rgba(0x8c959f)
		instance.outlineVariant = rgba(0xd0d7de)
		instance.primary = rgba(0x0969da)
		instance.onPrimary = rgba(0xffffff)
		instance.primaryContainer = rgba(0xddf4ff)
		instance.onPrimaryContainer = rgba(0x0969da)
		instance.inversePrimary = rgba(0x58a6ff)
		instance.secondary = rgba(0x57606a)
		instance.onSecondary = rgba(0xffffff)
		instance.secondaryContainer = rgba(0xebf0f4)
		instance.onSecondaryContainer = rgba(0x24292f)
		instance.tertiary = rgba(0x8250df)
		instance.onTertiary = rgba(0xffffff)
		instance.tertiaryContainer = rgba(0xfbefff)
		instance.onTertiaryContainer = rgba(0x6639ba)
		instance.errorContainer = rgba(0xffebe9)
		instance.onErrorContainer = rgba(0xcf222e)
		instance.successContainer = rgba(0xdafbe1)
		instance.onSuccessContainer = rgba(0x1a7f37)
		instance.warningContainer = rgba(0xfff8c5)
		instance.onWarningContainer = rgba(0x7d4e00)

		// Subagents
		instance.strength = rgba(0xc2410c)
		instance.strengthContainer = rgba(0xffedd5)
		instance.onStrengthContainer = rgba(0x7c2d12)
		instance.agility = rgba(0x0f766e)
		instance.agilityContainer = rgba(0xccfbf1)
		instance.onAgilityContainer = rgba(0x115e59)
		instance.intelligence = rgba(0x7e22ce)
		instance.intelligenceContainer = rgba(0xede9fe)
		instance.onIntelligenceContainer = rgba(0x4c1d95)

		// Diffs
		instance.diffAdded = rgba(0x1a7f37)
		instance.diffAddedContainer = rgba(0xdafbe1)
		instance.onDiffAddedContainer = rgba(0x1a7f37)
		instance.diffDeleted = rgba(0xcf222e)
		instance.diffDeletedContainer = rgba(0xffebe9)
		instance.onDiffDeletedContainer = rgba(0xcf222e)
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
	shapeExtraSmall unit.Dp = 3
	shapeSmall      unit.Dp = 6
	shapeMedium     unit.Dp = 8
	shapeLarge      unit.Dp = 12
	shapeExtraLarge unit.Dp = 16
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
