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

	surface              color.NRGBA
	surfaceContainer     color.NRGBA
	surfaceContainerHigh color.NRGBA
	onSurface            color.NRGBA
	onSurfaceVariant     color.NRGBA
	primary              color.NRGBA
	onPrimary            color.NRGBA
	primaryContainer     color.NRGBA
	onPrimaryContainer   color.NRGBA
	secondaryContainer   color.NRGBA
	onSecondaryContainer color.NRGBA
	outlineVariant       color.NRGBA
	errorContainer       color.NRGBA
	onErrorContainer     color.NRGBA
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
		instance.surface = rgba(0x1c1b1f)
		instance.surfaceContainer = rgba(0x211f26)
		instance.surfaceContainerHigh = rgba(0x2b2930)
		instance.onSurface = rgba(0xe6e1e5)
		instance.onSurfaceVariant = rgba(0xcac4d0)
		instance.primary = rgba(0xd0bcff)
		instance.onPrimary = rgba(0x381e72)
		instance.primaryContainer = rgba(0x4f378b)
		instance.onPrimaryContainer = rgba(0xeaddff)
		instance.secondaryContainer = rgba(0x633b48)
		instance.onSecondaryContainer = rgba(0xffd8e4)
		instance.outlineVariant = rgba(0x49454f)
		instance.errorContainer = rgba(0x8c1d18)
		instance.onErrorContainer = rgba(0xf9dedc)
	} else {
		instance.surface = rgba(0xfffbfe)
		instance.surfaceContainer = rgba(0xf3edf7)
		instance.surfaceContainerHigh = rgba(0xe8def8)
		instance.onSurface = rgba(0x1c1b1f)
		instance.onSurfaceVariant = rgba(0x49454f)
		instance.primary = rgba(0x6750a4)
		instance.onPrimary = rgba(0xffffff)
		instance.primaryContainer = rgba(0xe8def8)
		instance.onPrimaryContainer = rgba(0x21005d)
		instance.secondaryContainer = rgba(0xf3dfe2)
		instance.onSecondaryContainer = rgba(0x7d5260)
		instance.outlineVariant = rgba(0xcac4d0)
		instance.errorContainer = rgba(0xf9dedc)
		instance.onErrorContainer = rgba(0x410e0b)
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
	textDisplaySmall  unit.Sp = 36
	textHeadlineSmall unit.Sp = 24
	textTitleMedium   unit.Sp = 16
	textBodyLarge     unit.Sp = 16
	textBodyMedium    unit.Sp = 14
	textLabelLarge    unit.Sp = 14
	textLabelMedium   unit.Sp = 12
)
