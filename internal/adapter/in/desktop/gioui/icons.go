//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

type iconKind uint8

const (
	iconSidebar iconKind = iota
	iconInspector
	iconSearch
	iconAdd
	iconSend
	iconStop
	iconClose
	iconChevronDown
	iconChevronRight
	iconChevronUp
	iconKebab
	iconStar
	iconSettings
	iconCompose
	iconFolder
	iconArchive
	iconDiscord
	iconBrandLogo
)

func (s *shell) layoutActionIcon(gtx layout.Context, kind iconKind, size unit.Dp, color color.NRGBA) layout.Dimensions {
	px := gtx.Dp(size)
	gtx.Constraints.Min = image.Pt(px, px)
	gtx.Constraints.Max = image.Pt(px, px)
	paint.FillShape(gtx.Ops, color, clip.Stroke{Path: desktopIconPath(gtx.Ops, kind, float32(px)), Width: max(1, float32(gtx.Dp(1.5)))}.Op())
	return layout.Dimensions{Size: image.Pt(px, px)}
}

func desktopIconPath(ops *op.Ops, kind iconKind, size float32) clip.PathSpec {
	var path clip.Path
	path.Begin(ops)
	p := func(x, y float32) f32.Point { return f32.Pt(x*size, y*size) }
	line := func(x1, y1, x2, y2 float32) {
		path.MoveTo(p(x1, y1))
		path.LineTo(p(x2, y2))
	}
	switch kind {
	case iconSidebar, iconInspector:
		path.MoveTo(p(.16, .2))
		path.LineTo(p(.84, .2))
		path.LineTo(p(.84, .8))
		path.LineTo(p(.16, .8))
		path.Close()
		divider := float32(.39)
		if kind == iconInspector {
			divider = .61
		}
		line(divider, .2, divider, .8)
	case iconSearch:
		path.MoveTo(p(.48, .17))
		path.ArcTo(p(.48, .48), p(.29, .29), 5.5)
		line(.68, .68, .87, .87)
	case iconAdd:
		line(.5, .2, .5, .8)
		line(.2, .5, .8, .5)
	case iconSend:
		line(.5, .84, .5, .18)
		line(.5, .18, .22, .46)
		line(.5, .18, .78, .46)
	case iconStop:
		path.MoveTo(p(.34, .34))
		path.LineTo(p(.66, .34))
		path.LineTo(p(.66, .66))
		path.LineTo(p(.34, .66))
		path.Close()
	case iconClose:
		line(.25, .25, .75, .75)
		line(.75, .25, .25, .75)
	case iconChevronDown:
		line(.22, .38, .50, .66)
		line(.50, .66, .78, .38)
	case iconChevronRight:
		line(.38, .22, .66, .50)
		line(.66, .50, .38, .78)
	case iconChevronUp:
		line(.22, .66, .50, .38)
		line(.50, .38, .78, .66)
	case iconKebab:
		dot := func(y float32) {
			path.MoveTo(p(.42, y-.06))
			path.LineTo(p(.58, y-.06))
			path.LineTo(p(.58, y+.06))
			path.LineTo(p(.42, y+.06))
			path.Close()
		}
		dot(.26)
		dot(.50)
		dot(.74)
	case iconStar:
		path.MoveTo(p(.50, .14))
		path.LineTo(p(.61, .37))
		path.LineTo(p(.86, .37))
		path.LineTo(p(.66, .53))
		path.LineTo(p(.74, .79))
		path.LineTo(p(.50, .63))
		path.LineTo(p(.26, .79))
		path.LineTo(p(.34, .53))
		path.LineTo(p(.14, .37))
		path.LineTo(p(.39, .37))
		path.Close()
	case iconSettings:
		line(.18, .35, .82, .35)
		line(.38, .24, .38, .46)
		line(.18, .65, .82, .65)
		line(.62, .54, .62, .76)
	case iconCompose:
		path.MoveTo(p(.52, .2))
		path.LineTo(p(.2, .2))
		path.LineTo(p(.2, .8))
		path.LineTo(p(.8, .8))
		path.LineTo(p(.8, .48))
		line(.42, .58, .78, .22)
		line(.68, .22, .78, .32)
	case iconFolder:
		path.MoveTo(p(.18, .32))
		path.LineTo(p(.42, .32))
		path.LineTo(p(.50, .42))
		path.LineTo(p(.82, .42))
		path.LineTo(p(.82, .76))
		path.LineTo(p(.18, .76))
		path.Close()
	case iconArchive:
		path.MoveTo(p(.18, .30))
		path.LineTo(p(.82, .30))
		path.LineTo(p(.82, .45))
		path.LineTo(p(.18, .45))
		path.Close()
		path.MoveTo(p(.24, .45))
		path.LineTo(p(.24, .76))
		path.LineTo(p(.76, .76))
		path.LineTo(p(.76, .45))
		line(.40, .58, .60, .58)
	case iconDiscord:
		path.MoveTo(p(.25, .35))
		path.LineTo(p(.75, .35))
		path.LineTo(p(.80, .68))
		path.LineTo(p(.68, .62))
		path.LineTo(p(.58, .68))
		path.LineTo(p(.50, .62))
		path.LineTo(p(.42, .68))
		path.LineTo(p(.32, .62))
		path.LineTo(p(.20, .68))
		path.Close()
		line(.36, .46, .38, .48)
		line(.64, .46, .62, .48)
	case iconBrandLogo:
		path.MoveTo(p(.22, .80))
		path.LineTo(p(.22, .20))
		path.LineTo(p(.68, .20))
		path.LineTo(p(.68, .52))
		path.LineTo(p(.22, .52))
		line(.38, .36, .52, .36)
	}
	return path.End()
}

func (s *shell) layoutIconActionButton(gtx layout.Context, button *widget.Clickable, kind iconKind, tooltip string, action func()) layout.Dimensions {
	if button.Clicked(gtx) && action != nil {
		action()
	}
	size := gtx.Dp(30)
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(tooltip).Add(gtx.Ops)
	background := color.NRGBA{}
	foreground := s.theme.onSurfaceVariant
	if button.Hovered() {
		background = s.theme.surfaceContainerHigh
		foreground = s.theme.onSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutActionIcon(gtx, kind, 16, foreground)
			})
		})
	})
	if gtx.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}
