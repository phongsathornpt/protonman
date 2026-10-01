//go:build desktop || desktop_gio

package uikit

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Colors is the full theme palette handed to components.
//
// It is a superset of what any single component draws: components read the
// tokens they need and ignore the rest. Keeping one definition means the shell
// publishes the theme once instead of reshaping it per component package, and a
// token added to the theme becomes available everywhere without a per-package
// struct edit.
type Colors struct {
	Surface                 color.NRGBA
	SurfaceDim              color.NRGBA
	SurfaceBright           color.NRGBA
	SurfaceContainerLowest  color.NRGBA
	SurfaceContainerLow     color.NRGBA
	SurfaceContainer        color.NRGBA
	SurfaceContainerHigh    color.NRGBA
	SurfaceContainerHighest color.NRGBA

	OnSurface        color.NRGBA
	OnSurfaceVariant color.NRGBA

	Outline        color.NRGBA
	OutlineVariant color.NRGBA

	Primary            color.NRGBA
	OnPrimary          color.NRGBA
	PrimaryContainer   color.NRGBA
	OnPrimaryContainer color.NRGBA
	InversePrimary     color.NRGBA

	Secondary            color.NRGBA
	OnSecondary          color.NRGBA
	SecondaryContainer   color.NRGBA
	OnSecondaryContainer color.NRGBA

	Tertiary            color.NRGBA
	OnTertiary          color.NRGBA
	TertiaryContainer   color.NRGBA
	OnTertiaryContainer color.NRGBA

	ErrorContainer   color.NRGBA
	OnErrorContainer color.NRGBA

	SuccessContainer   color.NRGBA
	OnSuccessContainer color.NRGBA

	WarningContainer   color.NRGBA
	OnWarningContainer color.NRGBA

	// Subagent profile accents (Dota-style STR, AGI, INT).
	Strength                color.NRGBA
	StrengthContainer       color.NRGBA
	OnStrengthContainer     color.NRGBA
	Agility                 color.NRGBA
	AgilityContainer        color.NRGBA
	OnAgilityContainer      color.NRGBA
	Intelligence            color.NRGBA
	IntelligenceContainer   color.NRGBA
	OnIntelligenceContainer color.NRGBA

	// Diff rendering.
	DiffAdded              color.NRGBA
	DiffAddedContainer     color.NRGBA
	OnDiffAddedContainer   color.NRGBA
	DiffDeleted            color.NRGBA
	DiffDeletedContainer   color.NRGBA
	OnDiffDeletedContainer color.NRGBA
}

// Chrome supplies the palette plus the host-owned rendering primitives.
//
// The delegates keep theme policy and drawing code in the shell while letting
// components lay out pixels. Any delegate may be nil when a component does not
// call it; components must not assume a delegate exists beyond the ones they
// actually invoke.
type Chrome struct {
	Colors   Colors
	Material *material.Theme

	Label          func(layout.Context, string, unit.Sp, font.Weight, color.NRGBA, int) layout.Dimensions
	ActionIcon     func(layout.Context, Icon, unit.Dp, color.NRGBA) layout.Dimensions
	RoundedSurface func(layout.Context, unit.Dp, color.NRGBA, layout.Widget) layout.Dimensions
	BorderSurface  func(layout.Context, unit.Dp, color.NRGBA, color.NRGBA, int, layout.Widget) layout.Dimensions
	Divider        func(layout.Context) layout.Dimensions
	Inset          func(layout.Context, layout.Inset, layout.Widget) layout.Dimensions
	Button         func(layout.Context, *widget.Clickable, string, bool, func()) layout.Dimensions
	PrimaryButton  func(layout.Context, *widget.Clickable, string, bool, func()) layout.Dimensions
	DangerButton   func(layout.Context, *widget.Clickable, string, bool, func()) layout.Dimensions
	CloseButton    func(layout.Context, *widget.Clickable, func()) layout.Dimensions
	PanelTitle     func(layout.Context, string) layout.Dimensions
	MiniIconButton func(layout.Context, *widget.Clickable, string, color.NRGBA) layout.Dimensions
	Editor         func(layout.Context, string, *widget.Editor, bool) layout.Dimensions
	StatusDot      func(layout.Context, color.NRGBA) layout.Dimensions
	SettingsIcon   func(layout.Context, color.NRGBA) layout.Dimensions
	MonoFont       func(font.Weight) font.Font
}
