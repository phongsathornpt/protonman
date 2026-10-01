//go:build desktop || desktop_gio

package shell

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

// The shell's reusable layout primitives. Every view file builds on these
// through the Chrome struct in chrome.go, so the desktop has exactly one button,
// label, and surface implementation rather than one per feature.
func (s *Shell) layoutCenteredCard(gtx layout.Context, content layout.Widget) layout.Dimensions {
	if gtx.Constraints.Max.X > 0 {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
	}
	if gtx.Constraints.Max.Y > 0 && gtx.Constraints.Max.Y < 30000 {
		gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	}
	return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(680))
		return s.roundedSurface(gtx, shapeExtraLarge, s.theme.Colors.SurfaceContainer, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Top: 24, Bottom: 24, Left: 28, Right: 28}.Layout(gtx, content)
		})
	}))
}

func (s *Shell) layoutButton(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func()) layout.Dimensions {
	return s.layoutButtonStyle(gtx, button, label, enabled, action, false, false)
}

func (s *Shell) layoutPrimaryButton(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func()) layout.Dimensions {
	return s.layoutButtonStyle(gtx, button, label, enabled, action, true, false)
}

func (s *Shell) layoutDangerButton(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func()) layout.Dimensions {
	return s.layoutButtonStyle(gtx, button, label, enabled, action, false, true)
}

func (s *Shell) layoutButtonStyle(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func(), primary, danger bool) layout.Dimensions {
	if enabled && button.Clicked(gtx) && action != nil {
		action()
		gtx.Execute(op.InvalidateCmd{})
	}
	if !enabled {
		gtx = gtx.Disabled()
	}
	minHeight := gtx.Dp(34)
	inset := uikit.Inset{Top: 6, Bottom: 6, Left: 12, Right: 12}
	if primary || danger {
		minHeight = gtx.Dp(40)
		inset = uikit.Inset{Top: 8, Bottom: 8, Left: 16, Right: 16}
	}
	gtx.Constraints.Min.Y = minHeight
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = minHeight
		semantic.Button.Add(gtx.Ops)
		semantic.EnabledOp(gtx.Enabled()).Add(gtx.Ops)
		semantic.DescriptionOp(label).Add(gtx.Ops)
		background := s.theme.Colors.SecondaryContainer
		foreground := s.theme.Colors.OnSecondaryContainer
		if primary {
			background = s.theme.Colors.Primary
			foreground = s.theme.Colors.OnPrimary
		} else if danger {
			background = s.theme.Colors.ErrorContainer
			foreground = s.theme.Colors.OnErrorContainer
		}
		if !gtx.Enabled() {
			background = s.theme.Colors.SurfaceContainerHigh
			foreground = s.theme.Colors.OnSurfaceVariant
		} else if button.Hovered() && !primary && !danger {
			background = s.theme.Colors.PrimaryContainer
			foreground = s.theme.Colors.OnPrimaryContainer
		}
		return s.roundedSurface(gtx, shapeMedium, background, func(gtx layout.Context) layout.Dimensions {
			return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if strings.HasSuffix(label, " ▾") || strings.HasSuffix(label, " ▴") {
					clean := strings.TrimSuffix(strings.TrimSuffix(label, " ▾"), " ▴")
					chevronKind := uikit.KindChevronDown
					if strings.HasSuffix(label, " ▴") {
						chevronKind = uikit.KindChevronUp
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, clean, textLabelMedium, font.SemiBold, foreground, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Left: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return uikit.LayoutActionIcon(gtx, chevronKind, 11, foreground)
							})
						}),
					)
				}
				return s.layoutLabel(gtx, label, textLabelLarge, font.SemiBold, foreground, 1)
			})
		})
	})
	if enabled && gtx.Focused(button) {
		widget.Border{Color: s.theme.Colors.Primary, CornerRadius: shapeMedium, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *Shell) layoutIconButton(gtx layout.Context, button *widget.Clickable, icon, tooltip string, action func()) layout.Dimensions {
	kind, ok := desktopIconKind(icon)
	if ok {
		return s.layoutIconActionButton(gtx, button, kind, tooltip, action)
	}
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
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, icon, textTitleMedium, font.Normal, foreground, 1)
			}))
		})
	})
	if gtx.Focused(button) {
		widget.Border{Color: s.theme.Colors.Primary, CornerRadius: shapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *Shell) layoutAgentCapsuleButton(gtx layout.Context, button *widget.Clickable, label string, action func()) layout.Dimensions {
	if button.Clicked(gtx) && action != nil {
		action()
	}
	gtx.Constraints.Min.Y = gtx.Dp(28)
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(label).Add(gtx.Ops)
	background := s.theme.Colors.SurfaceContainerHigh
	foreground := s.theme.Colors.OnSurface
	if button.Hovered() {
		background = s.theme.Colors.SurfaceContainerHighest
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedSurface(gtx, shapeMedium, background, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Top: 4, Bottom: 4, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if strings.HasSuffix(label, " ▾") || strings.HasSuffix(label, " ▴") {
					clean := strings.TrimSuffix(strings.TrimSuffix(label, " ▾"), " ▴")
					chevronKind := uikit.KindChevronDown
					if strings.HasSuffix(label, " ▴") {
						chevronKind = uikit.KindChevronUp
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, clean, textLabelMedium, font.SemiBold, foreground, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return uikit.LayoutActionIcon(gtx, chevronKind, 10, foreground)
							})
						}),
					)
				}
				return s.layoutLabel(gtx, label, textLabelMedium, font.SemiBold, foreground, 1)
			})
		})
	})
	if gtx.Focused(button) {
		widget.Border{Color: s.theme.Colors.Primary, CornerRadius: shapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *Shell) layoutLabel(gtx layout.Context, value string, size unit.Sp, weight font.Weight, color color.NRGBA, maxLines int) layout.Dimensions {
	material := op.Record(gtx.Ops)
	paint.ColorOp{Color: color}.Add(gtx.Ops)
	return widget.Label{MaxLines: maxLines}.Layout(gtx, s.theme.Material.Shaper, s.theme.TextFont(weight), size, value, material.Stop())
}

func (s *Shell) roundedBorderSurface(gtx layout.Context, radius unit.Dp, background color.NRGBA, borderColor color.NRGBA, borderWidth int, content layout.Widget) layout.Dimensions {
	dims := s.roundedSurface(gtx, radius, background, content)
	if borderWidth > 0 && dims.Size.X > 0 && dims.Size.Y > 0 {
		widget.Border{Color: borderColor, CornerRadius: radius, Width: unit.Dp(borderWidth)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *Shell) roundedSurface(gtx layout.Context, radius unit.Dp, background color.NRGBA, content layout.Widget) layout.Dimensions {
	material := op.Record(gtx.Ops)
	dims := content(gtx)
	call := material.Stop()
	cornerRadius := min(gtx.Dp(radius), dims.Size.X/2, dims.Size.Y/2)
	stack := clip.RRect{
		Rect: image.Rectangle{Max: dims.Size},
		SE:   cornerRadius,
		SW:   cornerRadius,
		NW:   cornerRadius,
		NE:   cornerRadius,
	}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, background)
	call.Add(gtx.Ops)
	stack.Pop()
	return dims
}
