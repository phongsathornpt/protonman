//go:build desktop || desktop_gio

package shell

import (
	"image"
	"image/color"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

func (s *Shell) layoutIconActionButton(gtx layout.Context, button *widget.Clickable, kind uikit.IconKind, tooltip string, action func()) layout.Dimensions {
	if button.Clicked(gtx) && action != nil {
		action()
		gtx.Execute(op.InvalidateCmd{})
	}
	size := gtx.Dp(30)
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(tooltip).Add(gtx.Ops)
	background := color.NRGBA{}
	foreground := s.theme.Colors.OnSurfaceVariant
	if button.Hovered() {
		background = s.theme.Colors.SurfaceContainerHigh
		foreground = s.theme.Colors.OnSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, uikit.ShapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return uikit.LayoutActionIcon(gtx, kind, 16, foreground)
			})
		})
	})
	if gtx.Focused(button) {
		widget.Border{Color: s.theme.Colors.Primary, CornerRadius: uikit.ShapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}
