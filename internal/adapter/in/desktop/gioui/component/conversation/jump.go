//go:build desktop || desktop_gio

package conversation

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/unit"
)

type JumpChrome struct {
	SurfaceContainerHighest, PrimaryContainer, OutlineVariant color.NRGBA
	Primary, OnPrimaryContainer                               color.NRGBA
	Icon                                                      func(layout.Context, color.NRGBA) layout.Dimensions
	Label                                                     func(layout.Context, string, unit.Sp, font.Weight, color.NRGBA, int) layout.Dimensions
	RoundedSurface                                            func(layout.Context, unit.Dp, color.NRGBA, layout.Widget) layout.Dimensions
	BorderSurface                                             func(layout.Context, unit.Dp, color.NRGBA, color.NRGBA, int, layout.Widget) layout.Dimensions
}

func (c *Component) LayoutJumpToBottom(gtx layout.Context, chrome JumpChrome, onJump func()) layout.Dimensions {
	button := &c.jumpToBottom
	if button.Clicked(gtx) && onJump != nil {
		onJump()
	}
	background, border, foreground := chrome.SurfaceContainerHighest, chrome.OutlineVariant, chrome.Primary
	if button.Hovered() {
		background, border, foreground = chrome.PrimaryContainer, chrome.Primary, chrome.OnPrimaryContainer
	}
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("Jump to bottom of conversation").Add(gtx.Ops)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(9999), background, border, 1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 6, Bottom: 6, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return chrome.Icon(gtx, foreground) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, "Jump to bottom", unit.Sp(11), font.SemiBold, foreground, 1)
						})
					}),
				)
			})
		})
	})
}
