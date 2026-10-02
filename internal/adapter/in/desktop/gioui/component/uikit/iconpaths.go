//go:build desktop || desktop_gio

package uikit

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

func LayoutActionIcon(gtx layout.Context, kind IconKind, size unit.Dp, color color.NRGBA) layout.Dimensions {
	px := gtx.Dp(size)
	gtx.Constraints.Min = image.Pt(px, px)
	gtx.Constraints.Max = image.Pt(px, px)
	paint.FillShape(gtx.Ops, color, clip.Stroke{Path: desktopIconPath(gtx.Ops, kind, float32(px)), Width: max(1, float32(gtx.Dp(1.5)))}.Op())
	return layout.Dimensions{Size: image.Pt(px, px)}
}

func desktopIconPath(ops *op.Ops, kind IconKind, size float32) clip.PathSpec {
	var path clip.Path
	path.Begin(ops)
	p := func(x, y float32) f32.Point { return f32.Pt(x*size, y*size) }
	line := func(x1, y1, x2, y2 float32) {
		path.MoveTo(p(x1, y1))
		path.LineTo(p(x2, y2))
	}
	switch kind {
	case KindSidebar, KindInspector:
		path.MoveTo(p(.16, .2))
		path.LineTo(p(.84, .2))
		path.LineTo(p(.84, .8))
		path.LineTo(p(.16, .8))
		path.Close()
		divider := float32(.39)
		if kind == KindInspector {
			divider = .61
		}
		line(divider, .2, divider, .8)
	case KindSearch:
		path.MoveTo(p(.48, .17))
		path.ArcTo(p(.48, .48), p(.29, .29), 5.5)
		line(.68, .68, .87, .87)
	case KindAdd:
		line(.5, .2, .5, .8)
		line(.2, .5, .8, .5)
	case KindSend:
		line(.5, .84, .5, .18)
		line(.5, .18, .22, .46)
		line(.5, .18, .78, .46)
	case KindStop:
		path.MoveTo(p(.34, .34))
		path.LineTo(p(.66, .34))
		path.LineTo(p(.66, .66))
		path.LineTo(p(.34, .66))
		path.Close()
	case KindClose:
		line(.25, .25, .75, .75)
		line(.75, .25, .25, .75)
	case KindChevronDown:
		line(.22, .38, .50, .66)
		line(.50, .66, .78, .38)
	case KindChevronRight:
		line(.38, .22, .66, .50)
		line(.66, .50, .38, .78)
	case KindChevronUp:
		line(.22, .66, .50, .38)
		line(.50, .38, .78, .66)
	case KindKebab:
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
	case KindStar:
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
	case KindSettings:
		line(.18, .35, .82, .35)
		line(.38, .24, .38, .46)
		line(.18, .65, .82, .65)
		line(.62, .54, .62, .76)
	case KindCompose:
		path.MoveTo(p(.52, .2))
		path.LineTo(p(.2, .2))
		path.LineTo(p(.2, .8))
		path.LineTo(p(.8, .8))
		path.LineTo(p(.8, .48))
		line(.42, .58, .78, .22)
		line(.68, .22, .78, .32)
	case KindFolder:
		path.MoveTo(p(.18, .32))
		path.LineTo(p(.42, .32))
		path.LineTo(p(.50, .42))
		path.LineTo(p(.82, .42))
		path.LineTo(p(.82, .76))
		path.LineTo(p(.18, .76))
		path.Close()
	case KindArchive:
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
	case KindDiscord:
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
	case KindBrandLogo:
		path.MoveTo(p(.22, .80))
		path.LineTo(p(.22, .20))
		path.LineTo(p(.68, .20))
		path.LineTo(p(.68, .52))
		path.LineTo(p(.22, .52))
		line(.38, .36, .52, .36)
	case KindTerminal:
		line(.22, .26, .50, .50)
		line(.50, .50, .22, .74)
		line(.56, .74, .82, .74)
	case KindCheck:
		line(.22, .52, .44, .74)
		line(.44, .74, .82, .26)
	case KindCopy:
		path.MoveTo(p(.35, .20))
		path.LineTo(p(.78, .20))
		path.LineTo(p(.78, .65))
		path.LineTo(p(.35, .65))
		path.Close()
		line(.22, .35, .22, .80)
		line(.22, .80, .65, .80)
	case KindTrash:
		line(.22, .28, .78, .28)
		line(.40, .28, .40, .20)
		line(.60, .28, .60, .20)
		path.MoveTo(p(.28, .28))
		path.LineTo(p(.72, .28))
		path.LineTo(p(.67, .82))
		path.LineTo(p(.33, .82))
		path.Close()
		line(.43, .40, .45, .70)
		line(.57, .40, .55, .70)
	}
	return path.End()
}
