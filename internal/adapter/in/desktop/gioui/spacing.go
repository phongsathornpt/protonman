//go:build desktop || desktop_gio

package gioui

import (
	"gioui.org/layout"
	"gioui.org/unit"
)

// Gio's Inset keeps its incoming minimum constraint inside the padding. For
// controls with a minimum touch size that makes the padding enlarge the control
// by another 12-24dp. Treat the minimum as an outer size instead.
type desktopInset layout.Inset

func (in desktopInset) Layout(gtx layout.Context, content layout.Widget) layout.Dimensions {
	left, right := gtx.Dp(in.Left), gtx.Dp(in.Right)
	top, bottom := gtx.Dp(in.Top), gtx.Dp(in.Bottom)
	gtx.Constraints.Min.X = max(0, gtx.Constraints.Min.X-left-right)
	gtx.Constraints.Min.Y = max(0, gtx.Constraints.Min.Y-top-bottom)
	return layout.Inset(in).Layout(gtx, content)
}

func desktopUniformInset(padding unit.Dp) desktopInset {
	return desktopInset{Top: padding, Right: padding, Bottom: padding, Left: padding}
}
