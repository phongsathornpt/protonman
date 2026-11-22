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

// iconStrokeWidthDp keeps glyph stroke weight proportional across sizes while
// staying legible at small action sizes.
const iconStrokeWidthDp = 1.6

func LayoutActionIcon(gtx layout.Context, kind IconKind, size unit.Dp, color color.NRGBA) layout.Dimensions {
	px := gtx.Dp(size)
	gtx.Constraints.Min = image.Pt(px, px)
	gtx.Constraints.Max = image.Pt(px, px)
	width := float32(gtx.Dp(iconStrokeWidthDp))
	if px > 0 {
		minWidth := float32(px) * 0.0625
		if width < minWidth {
			width = minWidth
		}
		if width < 1 {
			width = 1
		}
	}
	paint.FillShape(gtx.Ops, color, clip.Stroke{Path: desktopIconPath(gtx.Ops, kind, float32(px)), Width: width}.Op())
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
		path.MoveTo(p(.18, .22))
		path.LineTo(p(.82, .22))
		path.LineTo(p(.82, .78))
		path.LineTo(p(.18, .78))
		path.Close()
		divider := float32(.40)
		if kind == KindInspector {
			divider = .60
		}
		line(divider, .22, divider, .78)
	case KindSearch:
		// Stroked circle centered at (.45,.45) with radius .20, plus a handle.
		// ArcTo foci coincide at the center for a circle; sweep 2π from the
		// east point to close the ring.
		path.MoveTo(p(.65, .45))
		path.ArcTo(p(.45, .45), p(.45, .45), 6.283185)
		path.Close()
		line(.595, .595, .81, .81)
	case KindAdd:
		line(.5, .22, .5, .78)
		line(.22, .5, .78, .5)
	case KindSend:
		path.MoveTo(p(.20, .52))
		path.LineTo(p(.80, .20))
		path.LineTo(p(.72, .72))
		path.LineTo(p(.52, .60))
		path.Close()
		line(.52, .60, .80, .20)
	case KindStop:
		path.MoveTo(p(.36, .36))
		path.LineTo(p(.64, .36))
		path.LineTo(p(.64, .64))
		path.LineTo(p(.36, .64))
		path.Close()
	case KindClose:
		line(.28, .28, .72, .72)
		line(.72, .28, .28, .72)
	case KindChevronDown:
		line(.26, .40, .50, .62)
		line(.50, .62, .74, .40)
	case KindChevronRight:
		line(.40, .26, .62, .50)
		line(.62, .50, .40, .74)
	case KindChevronUp:
		line(.26, .60, .50, .38)
		line(.50, .38, .74, .60)
	case KindKebab:
		dot := func(y float32) {
			path.MoveTo(p(.43, y-.055))
			path.LineTo(p(.57, y-.055))
			path.LineTo(p(.57, y+.055))
			path.LineTo(p(.43, y+.055))
			path.Close()
		}
		dot(.28)
		dot(.50)
		dot(.72)
	case KindStar:
		path.MoveTo(p(.50, .16))
		path.LineTo(p(.60, .385))
		path.LineTo(p(.84, .385))
		path.LineTo(p(.65, .535))
		path.LineTo(p(.72, .78))
		path.LineTo(p(.50, .64))
		path.LineTo(p(.28, .78))
		path.LineTo(p(.35, .535))
		path.LineTo(p(.16, .385))
		path.LineTo(p(.40, .385))
		path.Close()
	case KindSettings:
		line(.20, .36, .80, .36)
		path.MoveTo(p(.36, .28))
		path.LineTo(p(.46, .28))
		path.LineTo(p(.46, .44))
		path.LineTo(p(.36, .44))
		path.Close()
		line(.20, .64, .80, .64)
		path.MoveTo(p(.54, .56))
		path.LineTo(p(.64, .56))
		path.LineTo(p(.64, .72))
		path.LineTo(p(.54, .72))
		path.Close()
	case KindCompose:
		path.MoveTo(p(.48, .22))
		path.LineTo(p(.22, .22))
		path.LineTo(p(.22, .78))
		path.LineTo(p(.78, .78))
		path.LineTo(p(.78, .52))
		path.LineTo(p(.70, .54))
		path.LineTo(p(.70, .70))
		path.LineTo(p(.30, .70))
		path.LineTo(p(.30, .30))
		path.LineTo(p(.46, .30))
		path.Close()
		line(.42, .58, .76, .24)
		line(.60, .24, .76, .24)
		line(.76, .24, .76, .40)
	case KindFolder:
		path.MoveTo(p(.20, .32))
		path.LineTo(p(.42, .32))
		path.LineTo(p(.48, .40))
		path.LineTo(p(.80, .40))
		path.LineTo(p(.80, .74))
		path.LineTo(p(.20, .74))
		path.Close()
	case KindArchive:
		path.MoveTo(p(.20, .28))
		path.LineTo(p(.80, .28))
		path.LineTo(p(.80, .42))
		path.LineTo(p(.20, .42))
		path.Close()
		path.MoveTo(p(.26, .42))
		path.LineTo(p(.26, .74))
		path.LineTo(p(.74, .74))
		path.LineTo(p(.74, .42))
		path.Close()
		line(.40, .46, .40, .50)
		line(.60, .46, .60, .50)
		line(.42, .60, .58, .60)
	case KindDiscord:
		path.MoveTo(p(.22, .32))
		path.LineTo(p(.78, .32))
		path.LineTo(p(.82, .66))
		path.LineTo(p(.70, .60))
		path.LineTo(p(.60, .68))
		path.LineTo(p(.54, .62))
		path.LineTo(p(.46, .62))
		path.LineTo(p(.40, .68))
		path.LineTo(p(.30, .60))
		path.LineTo(p(.18, .66))
		path.Close()
		line(.37, .45, .37, .49)
		line(.63, .45, .63, .49)
	case KindBrandLogo:
		path.MoveTo(p(.24, .78))
		path.LineTo(p(.24, .22))
		path.LineTo(p(.66, .22))
		path.LineTo(p(.66, .50))
		path.LineTo(p(.24, .50))
		path.Close()
		line(.38, .36, .52, .36)
		line(.38, .60, .58, .60)
		line(.38, .68, .52, .68)
	case KindTerminal:
		line(.26, .30, .50, .50)
		line(.50, .50, .26, .70)
		line(.58, .70, .78, .70)
	case KindCheck:
		line(.24, .53, .44, .72)
		line(.44, .72, .78, .28)
	case KindCopy:
		path.MoveTo(p(.36, .22))
		path.LineTo(p(.78, .22))
		path.LineTo(p(.78, .64))
		path.LineTo(p(.36, .64))
		path.Close()
		line(.22, .36, .22, .78)
		line(.22, .78, .64, .78)
	case KindTrash:
		line(.24, .28, .76, .28)
		line(.41, .28, .41, .21)
		line(.59, .28, .59, .21)
		path.MoveTo(p(.30, .28))
		path.LineTo(p(.70, .28))
		path.LineTo(p(.66, .79))
		path.LineTo(p(.34, .79))
		path.Close()
		line(.44, .41, .45, .69)
		line(.56, .41, .55, .69)
	}
	return path.End()
}
