//go:build desktop || desktop_gio

// Package uikit holds the design tokens and layout primitives shared by every
// Gio desktop component package.
//
// It is a leaf: it must not import any other component package, the shell, or
// any internal project package. Components embed uikit.Chrome for the common
// visual surface and declare only their own extras alongside it, so a change to
// a shape, text size, or primitive implementation lands in exactly one place.
package uikit

import (
	"gioui.org/layout"
	"gioui.org/unit"
)

// Shape tokens. Corner radii are the only radius vocabulary the desktop uses.
const (
	ShapeNone       unit.Dp = 0
	ShapeExtraSmall unit.Dp = 3
	ShapeSmall      unit.Dp = 6
	ShapeMedium     unit.Dp = 8
	ShapeLarge      unit.Dp = 12
	ShapeExtraLarge unit.Dp = 16
	ShapeFull       unit.Dp = 9999
)

// Text tokens, following the Material type scale names the desktop mirrors.
const (
	TextDisplaySmall   unit.Sp = 30
	TextHeadlineMedium unit.Sp = 24
	TextHeadlineSmall  unit.Sp = 20
	TextTitleLarge     unit.Sp = 18
	TextTitleMedium    unit.Sp = 16
	TextTitleSmall     unit.Sp = 14
	TextBodyLarge      unit.Sp = 16
	TextBodyMedium     unit.Sp = 14
	TextBodySmall      unit.Sp = 12
	TextLabelLarge     unit.Sp = 14
	TextLabelMedium    unit.Sp = 12
	TextLabelSmall     unit.Sp = 11
)

// Inset applies padding and relaxes the minimum constraints so padded content
// still fits inside a fixed allocation. Gio's layout.Inset only insets the
// maximum, which collapses nested flexible children, so components share this
// variant instead of reimplementing it.
type Inset layout.Inset

// Layout applies the inset and returns the resulting dimensions.
func (in Inset) Layout(gtx layout.Context, content layout.Widget) layout.Dimensions {
	left, right := gtx.Dp(in.Left), gtx.Dp(in.Right)
	top, bottom := gtx.Dp(in.Top), gtx.Dp(in.Bottom)
	gtx.Constraints.Min.X = max(0, gtx.Constraints.Min.X-left-right)
	gtx.Constraints.Min.Y = max(0, gtx.Constraints.Min.Y-top-bottom)
	return layout.Inset(in).Layout(gtx, content)
}

// UniformInset builds an Inset with the same padding on all four edges.
func UniformInset(padding unit.Dp) Inset {
	return Inset{Top: padding, Right: padding, Bottom: padding, Left: padding}
}
