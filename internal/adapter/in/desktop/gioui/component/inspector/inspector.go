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
	PanelWidth unit.Dp = 336
)

// tabLabels is the single source of truth for inspector tabs. tabButtons must
// stay the same length; a fourth slot would silently swallow input.
var tabLabels = [...]string{"Plan", "Memory", "Skills"}

type Snapshot struct {
	Connected bool
}

// Chrome is the shared uikit surface; the inspector needs no extra primitives.
type Chrome = uikit.Chrome

type ViewInput struct {
	Snapshot Snapshot
	Chrome   Chrome
	Panel    func(layout.Context, int, int) layout.Dimensions
	// Width overrides the default panel allocation. Shell sets it from the
	// docked-width computation so the conversation-minimum cap reaches the
	// screen instead of being overwritten by the default width.
	Width unit.Dp
}

type Component struct {
	list          layout.List
	tabButtons    [len(tabLabels)]widget.Clickable
	activeTab     int
	visible       bool
	visibilitySet bool
	skillSearch   widget.Editor
	skillButtons  map[string]*widget.Clickable
}

func New() *Component { return &Component{list: layout.List{Axis: layout.Vertical}} }

// ShouldShowForWidth reports the default inspector visibility for a full
// window width: visible only when the sidebar, divider, a usable conversation
// pane, and the panel all fit. Unlike ShouldShow, it measures the whole
// window instead of one pane's leftover constraints.
func (c *Component) ShouldShowForWidth(windowWidth unit.Dp, sidebarVisible bool) bool {
	if c.visibilitySet {
		return c.visible
	}
	return Docked(windowWidth, sidebarVisible)
}

// Docked reports whether the inspector fits beside the conversation pane at
// the given full window width: sidebar plus divider plus a usable
// conversation width plus the inspector panel. Callers computing this from
// leftover constraints inside the main pane must pass the full window width,
// not the post-sidebar remainder, or narrow windows will dock a panel that
// collapses the conversation to a sliver.
func Docked(windowWidth unit.Dp, sidebarVisible bool) bool {
	sidebar := uikit.SidebarWidth
	if windowWidth < uikit.SidebarCompactBreakpoint {
		sidebar = uikit.SidebarWidthCompact
	}
	needed := uikit.InspectorMinWidth + uikit.ConversationMinWidth + uikit.InspectorDividerWidth
	if sidebarVisible {
		needed += sidebar + uikit.InspectorDividerWidth
	}
	return windowWidth >= needed
}

// ToggleForWidth flips the current visibility under the same whole-window
// rule ShouldShowForWidth uses, so the Cmd/Ctrl+I shortcut and the topbar
// button agree with the automatic dock decision.
func (c *Component) ToggleForWidth(windowWidth unit.Dp, sidebarVisible bool) {
	c.visible = !c.ShouldShowForWidth(windowWidth, sidebarVisible)
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

func (c *Component) SetActiveTab(tab int) { c.activeTab = max(0, min(tab, len(tabLabels)-1)) }

func (c *Component) Layout(gtx layout.Context, input ViewInput, compact bool) layout.Dimensions {
	if compact {
		gtx.Constraints.Min.X = 0
	} else if input.Width > 0 {
		gtx.Constraints.Min.X = gtx.Dp(input.Width)
		gtx.Constraints.Max.X = gtx.Dp(input.Width)
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

func (c *Component) PanelCount(_ Snapshot) int {
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
				return layout.Inset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return c.layoutTabs(gtx, chrome) })
			}),
		)
	})
}

func (c *Component) layoutTabs(gtx layout.Context, chrome Chrome) layout.Dimensions {
	tabs := tabLabels
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
