//go:build desktop || desktop_gio

package settings

import (
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

type MCPIntegration struct {
	Name, Command string
	Args, Env     []string
}

// LayoutMCPIntegrationRow owns card presentation, selection, and row click state.
func (c *Component) LayoutMCPIntegrationRow(gtx layout.Context, item MCPIntegration, selected, enabled bool, chrome Chrome) layout.Dimensions {
	mcp := &c.mcp
	button := mcp.IntegrationButtons[item.Name]
	if button == nil {
		if mcp.IntegrationButtons == nil {
			mcp.IntegrationButtons = make(map[string]*widget.Clickable)
		}
		button = new(widget.Clickable)
		mcp.IntegrationButtons[item.Name] = button
	}
	rowContext := gtx
	if !enabled {
		rowContext = gtx.Disabled()
	}
	if enabled && button.Clicked(rowContext) {
		mcp.SelectedName = item.Name
		mcp.EditorKey = ""
		mcp.FormVisible = true
		mcp.ConfirmDelete = false
		gtx.Execute(op.InvalidateCmd{})
	}
	gtx.Constraints.Min.Y = gtx.Dp(52)
	background, border, borderWidth := chrome.Colors.Surface, chrome.Colors.OutlineVariant, 1
	if selected {
		background, border, borderWidth = chrome.Colors.SurfaceContainerHigh, chrome.Colors.Primary, 2
	} else if button.Hovered() {
		background = chrome.Colors.SurfaceContainerHigh
	}
	command := item.Command
	if len(item.Args) > 0 {
		command += " " + strings.Join(item.Args, " ")
	}
	dims := button.Layout(rowContext, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		semantic.DescriptionOp("Edit MCP integration " + item.Name).Add(gtx.Ops)
		return chrome.Inset(gtx, layout.Inset{Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
			return chrome.BorderSurface(gtx, unit.Dp(8), background, border, borderWidth, func(gtx layout.Context) layout.Dimensions {
				return chrome.Inset(gtx, layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.Inset(gtx, layout.Inset{Right: 8}, func(gtx layout.Context) layout.Dimensions {
										size := gtx.Dp(8)
										gtx.Constraints.Min = image.Pt(size, size)
										gtx.Constraints.Max = image.Pt(size, size)
										paint.FillShape(gtx.Ops, chrome.Colors.Primary, clip.Ellipse{Max: image.Pt(size, size)}.Op(gtx.Ops))
										return layout.Dimensions{Size: image.Pt(size, size)}
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, item.Name, unit.Sp(14), font.SemiBold, chrome.Colors.OnSurface, 1)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
										return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
											return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}, func(gtx layout.Context) layout.Dimensions {
												return chrome.Label(gtx, "mcp stdio", unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 1)
											})
										})
									})
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Spacer{}.Layout(gtx) }),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, "Edit ✎", unit.Sp(11), font.Medium, chrome.Colors.OnSurfaceVariant, 1)
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return chrome.Inset(gtx, layout.Inset{Top: 4, Left: 14}, func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, command, unit.Sp(12), font.Normal, chrome.Colors.OnSurfaceVariant, 1)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if len(item.Env) == 0 {
								return layout.Dimensions{}
							}
							return chrome.Inset(gtx, layout.Inset{Top: 4, Left: 14}, func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, "env: "+strings.Join(item.Env, ", "), unit.Sp(11), font.Normal, chrome.Colors.Outline, 1)
							})
						}),
					)
				})
			})
		})
	})
	return dims
}
