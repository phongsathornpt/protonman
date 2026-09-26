//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestDesktopInsetKeepsOuterTouchTarget(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Min: image.Pt(100, 44), Max: image.Pt(200, 100)}, Metric: unit.Metric{PxPerDp: 1}}
	var inner layout.Constraints
	dims := (desktopInset{Top: 10, Bottom: 10, Left: 16, Right: 16}).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		inner = gtx.Constraints
		return layout.Dimensions{Size: inner.Min}
	})
	if inner.Min != (image.Pt(68, 24)) || dims.Size != (image.Pt(100, 44)) {
		t.Fatalf("inner min = %v, outer size = %v; want 68x24 inside a 100x44 control", inner.Min, dims.Size)
	}
}
