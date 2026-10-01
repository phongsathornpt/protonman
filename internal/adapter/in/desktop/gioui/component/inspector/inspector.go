//go:build desktop || desktop_gio

// Package inspector owns the inspector's tab, scroll, and visibility widgets.
// Shell supplies the current session snapshot and renders feature panels.
package inspector

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

const (
	WideBreakpoint unit.Dp = 960
	PanelWidth     unit.Dp = 336
)

type Snapshot struct {
	ExternalAgent bool
	Connected     bool
	AgentName     string
}

// Chrome is the shared uikit surface; the inspector needs no extra primitives.
type Chrome = uikit.Chrome

type ViewInput struct {
	Snapshot Snapshot
	Chrome   Chrome
	Panel    func(layout.Context, int, int) layout.Dimensions
}

type Component struct {
	list          layout.List
	tabButtons    [4]widget.Clickable
	activeTab     int
	visible       bool
	visibilitySet bool
	skillSearch   widget.Editor
	skillButtons  map[string]*widget.Clickable
}

func New() *Component { return &Component{list: layout.List{Axis: layout.Vertical}} }

func (c *Component) ShouldShow(wide bool) bool {
	if c.visibilitySet {
		return c.visible
	}
	return wide
}

func (c *Component) Toggle(wide bool) {
	c.visible = !c.ShouldShow(wide)
	c.visibilitySet = true
}

func (c *Component) SetVisible(visible bool) {
	c.visible = visible
	c.visibilitySet = true
}

func (c *Component) ResetVisibility() {
	c.visible = false
	c.visibilitySet = false
	c.list.Position = layout.Position{}
}

func (c *Component) ActiveTab() int { return c.activeTab }

func (c *Component) SetActiveTab(tab int) { c.activeTab = max(0, min(tab, 2)) }

func (c *Component) Layout(gtx layout.Context, input ViewInput, compact bool) layout.Dimensions {
	if compact {
		gtx.Constraints.Min.X = 0
	} else {
		gtx.Constraints.Min.X = gtx.Dp(PanelWidth)
		gtx.Constraints.Max.X = gtx.Dp(PanelWidth)
	}
	chrome := input.Chrome
	return chrome.RoundedSurface(gtx, 0, chrome.Colors.SurfaceDim, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.layoutHeader(gtx, input) }),
			layout.Rigid(chrome.Divider),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				count := c.PanelCount(input.Snapshot)
				return c.list.Layout(gtx, count, func(gtx layout.Context, index int) layout.Dimensions {
					return layout.Inset{Top: 6, Bottom: 6, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.BorderSurface(gtx, unit.Dp(12), chrome.Colors.SurfaceContainer, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
							semantic.DescriptionOp("Session inspector panel").Add(gtx.Ops)
							return layout.Inset{Top: 14, Bottom: 14, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								if input.Snapshot.ExternalAgent {
									return c.layoutExternalAgent(gtx, input.Snapshot.AgentName, chrome)
								}
								if input.Panel == nil {
									return layout.Dimensions{}
								}
								return input.Panel(gtx, c.activeTab, index)
							})
						})
					})
				})
			}),
		)
	})
}

func (c *Component) layoutExternalAgent(gtx layout.Context, agentName string, chrome Chrome) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "Agent capabilities", unit.Sp(16), font.SemiBold, chrome.Colors.OnSurface, 2)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "This conversation uses standard ACP chat, tools, permissions, cancellation, and reconnect behavior through "+agentName+".", unit.Sp(14), font.Normal, chrome.Colors.OnSurfaceVariant, 4)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "Goal, TODO, Memory, and runtime controls are hidden because they are Protonman extensions.", unit.Sp(14), font.Normal, chrome.Colors.OnSurfaceVariant, 4)
		}),
	)
}

func (c *Component) PanelCount(snapshot Snapshot) int {
	if snapshot.ExternalAgent {
		return 1
	}
	switch c.activeTab {
	case 1, 2:
		return 1
	default:
		return 3
	}
}

func (c *Component) layoutHeader(gtx layout.Context, input ViewInput) layout.Dimensions {
	chrome := input.Chrome
	status := "Waiting for agent"
	if input.Snapshot.Connected {
		status = "Agent connected"
	}
	return layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, "INSPECTOR", unit.Sp(11), font.Bold, chrome.Colors.OnSurfaceVariant, 1)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Spacer{}.Layout(gtx) }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, status, unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 1)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if input.Snapshot.ExternalAgent {
					return layout.Dimensions{}
				}
				return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return c.layoutTabs(gtx, chrome) })
			}),
		)
	})
}

func (c *Component) layoutTabs(gtx layout.Context, chrome Chrome) layout.Dimensions {
	tabs := [...]string{"Plan", "Memory", "Skills"}
	if c.activeTab >= len(tabs) {
		c.activeTab = 0
	}
	return chrome.RoundedSurface(gtx, unit.Dp(12), chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 2, Bottom: 2, Left: 2, Right: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(tabs))
			for i, label := range tabs {
				i, label := i, label
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					button := &c.tabButtons[i]
					if button.Clicked(gtx) {
						c.activeTab = i
					}
					active := c.activeTab == i
					bg, fg, weight := color.NRGBA{}, chrome.Colors.OnSurfaceVariant, font.Medium
					if active {
						bg, fg, weight = chrome.Colors.Surface, chrome.Colors.OnSurface, font.SemiBold
					} else if button.Hovered() {
						bg, fg = chrome.Colors.SurfaceContainerHigh, chrome.Colors.OnSurface
					}
					gtx.Constraints.Min.Y = gtx.Dp(26)
					return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.RoundedSurface(gtx, unit.Dp(6), bg, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: 3, Bottom: 3, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, label, unit.Sp(11), weight, fg, 1)
								})
							})
						})
					})
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}
