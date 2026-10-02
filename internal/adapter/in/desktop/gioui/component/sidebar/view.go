//go:build desktop || desktop_gio

package sidebar

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	sidebarSessionRowMinHeight = 44
	RowGlyphSize               = 14
	sidebarMetadataLineHeight  = 15
	RowIndent                  = 22
	sidebarGroupTopInset       = 8
	sidebarGroupBottomInset    = 2
)

func (c *Component) Layout(gtx layout.Context, input ViewInput) layout.Dimensions {
	input.Actions = withDefaultActions(input.Actions)
	c.view = input
	return c.layoutSidebar(gtx, input.Snapshot)
}

func (c *Component) LayoutDeleteModal(gtx layout.Context, input ViewInput) layout.Dimensions {
	input.Actions = withDefaultActions(input.Actions)
	c.view = input
	return c.layoutDeleteModal(gtx)
}

func (c *Component) FocusSearch(gtx layout.Context) {
	gtx.Execute(key.FocusCmd{Tag: &c.searchEditor})
}

func (c *Component) layoutSidebar(gtx layout.Context, snapshot Snapshot) layout.Dimensions {
	width := unit.Dp(260)
	if gtx.Constraints.Max.X < gtx.Dp(900) {
		width = 220
	}
	gtx.Constraints.Min.X = gtx.Dp(width)
	gtx.Constraints.Max.X = gtx.Dp(width)
	paint.FillShape(gtx.Ops, c.view.Chrome.Colors.SurfaceDim, clip.Rect{
		Max: image.Pt(gtx.Dp(width), gtx.Constraints.Max.Y),
	}.Op())

	nextMinute := gtx.Now.Truncate(time.Minute).Add(time.Minute)
	if !nextMinute.After(gtx.Now) {
		nextMinute = gtx.Now.Add(time.Minute)
	}
	gtx.Execute(op.InvalidateCmd{At: nextMinute})

	allRows := c.Rows(Model{State: snapshot.State, AgentProfiles: snapshot.AgentProfiles, Pinned: snapshot.PinnedSessions, CustomTitles: snapshot.CustomTitles, FilterMode: snapshot.FilterMode, Revision: snapshot.Revision})
	rows := c.DisplayRows(allRows, c.searchEditor.Text(), snapshot.FilterMode, snapshot.Revision)
	if snapshot.State.ActiveSessionID != "" && snapshot.State.ActiveSessionID != c.lastEnsuredSessionID {
		c.lastEnsuredSessionID = snapshot.State.ActiveSessionID
		c.EnsureVisible(snapshot.State.ActiveSessionID, rows)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.layoutSidebarHeader(gtx, snapshot, rows)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(rows) == 0 {
				emptyText := "No conversations yet. Start one from an existing workspace."
				hasFilter := false
				if strings.TrimSpace((&c.searchEditor).Text()) != "" {
					emptyText = "No conversations matching \"" + (&c.searchEditor).Text() + "\""
					hasFilter = true
				} else if snapshot.FilterMode == "running" {
					emptyText = "No running conversations."
					hasFilter = true
				} else if snapshot.FilterMode == "pinned" {
					emptyText = "No pinned conversations. Hover a session and press the star to pin it."
					hasFilter = true
				}
				if c.ClearFilterButton().Clicked(gtx) {
					(&c.searchEditor).SetText("")
					if c.view.Actions.SetFilterMode != nil {
						c.view.Actions.SetFilterMode("all")
					}
				}
				canCreate := snapshot.Connection == "connected" && !snapshot.CreatingSession
				if !hasFilter && c.NewSessionButton().Clicked(gtx) && canCreate && c.view.Actions.NewSession != nil {
					c.view.Actions.NewSession()
				}
				return uikit.Inset{Top: 24, Bottom: 24, Left: 16, Right: 16}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						children := []layout.FlexChild{
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return c.view.Chrome.Label(gtx, emptyText, uikit.TextBodySmall, font.Normal, c.view.Chrome.Colors.OnSurfaceVariant, 3)
							}),
						}
						if hasFilter {
							children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Top: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.Button(gtx, c.ClearFilterButton(), "Clear filter", true, nil)
								})
							}))
						} else {
							children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Top: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.Button(gtx, c.NewSessionButton(), "New conversation", canCreate, func() {
										if canCreate && c.view.Actions.NewSession != nil {
											c.view.Actions.NewSession()
										}
									})
								})
							}))
						}
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
					},
				)
			}
			return (&c.list).Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
				return c.layoutSidebarRow(gtx, rows[index], snapshot.State, rowNavContext{rows: rows, index: index})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.view.Chrome.Divider(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.layoutSidebarFooter(gtx, snapshot)
		}),
	)
}

func (c *Component) layoutSidebarFooter(gtx layout.Context, snapshot Snapshot) layout.Dimensions {
	if c.InspectorButton().Clicked(gtx) {
		c.view.Actions.OpenSettings()
		gtx.Execute(key.FocusCmd{Tag: nil})
	}

	if c.PinnedFilterButton().Clicked(gtx) && c.view.Actions.SetFilterMode != nil {
		if snapshot.FilterMode == "pinned" {
			c.view.Actions.SetFilterMode("all")
		} else {
			c.view.Actions.SetFilterMode("pinned")
		}
	}

	if c.RunningFilterButton().Clicked(gtx) && c.view.Actions.SetFilterMode != nil {
		if snapshot.FilterMode == "running" {
			c.view.Actions.SetFilterMode("all")
		} else {
			c.view.Actions.SetFilterMode("running")
		}
	}

	if c.CommunityButton().Clicked(gtx) {
		c.view.Actions.OpenCommunity()
	}

	statusColor := c.view.Chrome.Colors.OnSuccessContainer
	statusLabel := "Connected"
	switch snapshot.Connection {
	case "connected":
		statusColor = c.view.Chrome.Colors.OnSuccessContainer
		statusLabel = "Connected"
	case "connecting", "reconnecting":
		statusColor = c.view.Chrome.Colors.OnWarningContainer
		statusLabel = "Connecting"
	default:
		statusColor = c.view.Chrome.Colors.OnErrorContainer
		statusLabel = "Offline"
	}
	status := strings.ToLower(snapshot.Status)
	if strings.Contains(status, "failed") || (strings.Contains(status, "unavailable") && !strings.Contains(status, "session list unavailable")) || strings.Contains(status, "disconnected") {
		statusColor = c.view.Chrome.Colors.OnErrorContainer
		statusLabel = "Offline"
	}

	return uikit.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btnGtx := gtx
						bg := color.NRGBA{}
						fg := c.view.Chrome.Colors.OnSurfaceVariant
						if snapshot.SettingsModalOpen {
							bg = c.view.Chrome.Colors.PrimaryContainer
							fg = c.view.Chrome.Colors.OnPrimaryContainer
						} else if c.InspectorButton().Hovered() {
							bg = c.view.Chrome.Colors.SurfaceContainerHigh
							fg = c.view.Chrome.Colors.OnSurface
						}
						semantic.Button.Add(btnGtx.Ops)
						semantic.DescriptionOp("Open Settings (⌘,)").Add(btnGtx.Ops)
						return c.InspectorButton().Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
							return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
								return uikit.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.ActionIcon(gtx, uikit.IconSettings, 16, fg)
								})
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btnGtx := gtx
							bg := color.NRGBA{}
							fg := c.view.Chrome.Colors.OnSurfaceVariant
							if snapshot.FilterMode == "pinned" {
								bg = c.view.Chrome.Colors.PrimaryContainer
								fg = c.view.Chrome.Colors.OnPrimaryContainer
							} else if c.PinnedFilterButton().Hovered() {
								bg = c.view.Chrome.Colors.SurfaceContainerHigh
								fg = c.view.Chrome.Colors.OnSurface
							}
							semantic.Button.Add(btnGtx.Ops)
							semantic.DescriptionOp("Toggle pinned conversations filter").Add(btnGtx.Ops)
							return c.PinnedFilterButton().Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
								return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
									return uikit.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return c.view.Chrome.ActionIcon(gtx, uikit.IconStar, 16, fg)
									})
								})
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btnGtx := gtx
							bg := color.NRGBA{}
							fg := c.view.Chrome.Colors.OnSurfaceVariant
							if snapshot.FilterMode == "running" {
								bg = c.view.Chrome.Colors.PrimaryContainer
								fg = c.view.Chrome.Colors.OnPrimaryContainer
							} else if c.RunningFilterButton().Hovered() {
								bg = c.view.Chrome.Colors.SurfaceContainerHigh
								fg = c.view.Chrome.Colors.OnSurface
							}
							semantic.Button.Add(btnGtx.Ops)
							semantic.DescriptionOp("Toggle running tasks filter").Add(btnGtx.Ops)
							return c.RunningFilterButton().Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
								return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
									return uikit.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return c.view.Chrome.ActionIcon(gtx, uikit.IconTerminal, 16, fg)
									})
								})
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btnGtx := gtx
							bg := color.NRGBA{}
							fg := c.view.Chrome.Colors.OnSurfaceVariant
							if c.CommunityButton().Hovered() {
								bg = c.view.Chrome.Colors.SurfaceContainerHigh
								fg = c.view.Chrome.Colors.OnSurface
							}
							semantic.Button.Add(btnGtx.Ops)
							semantic.DescriptionOp("Open Protonman repository on GitHub").Add(btnGtx.Ops)
							return c.CommunityButton().Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
								return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
									return uikit.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return c.view.Chrome.ActionIcon(gtx, uikit.IconBrandLogo, 16, fg)
									})
								})
							})
						})
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeFull, statusColor, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min = image.Pt(gtx.Dp(6), gtx.Dp(6))
							gtx.Constraints.Max = image.Pt(gtx.Dp(6), gtx.Dp(6))
							return layout.Dimensions{Size: gtx.Constraints.Min}
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return c.view.Chrome.Label(gtx, statusLabel, uikit.TextLabelSmall, font.Normal, c.view.Chrome.Colors.OnSurfaceVariant, 1)
						})
					}),
				)
			}),
		)
	})
}

func (c *Component) layoutSidebarHeader(gtx layout.Context, snapshot Snapshot, rows []Row) layout.Dimensions {
	return uikit.Inset{Top: 14, Bottom: 4, Left: 12, Right: 12}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			canCreate := snapshot.Connection == "connected" && !snapshot.CreatingSession

			items := []layout.FlexChild{
				// Top bar: [P] Protonman ...... new-session icon action.
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Bottom: 8, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return uikit.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return c.view.Chrome.ActionIcon(gtx, uikit.IconBrandLogo, 20, c.view.Chrome.Colors.OnSurface)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return c.view.Chrome.Label(gtx, "Protonman", uikit.TextTitleMedium, font.Bold, c.view.Chrome.Colors.OnSurface, 1)
									}),
								)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := c.NewSessionButton()
								if btn.Clicked(gtx) && canCreate && c.view.Actions.NewSession != nil {
									c.view.Actions.NewSession()
								}
								bg := color.NRGBA{}
								fg := c.view.Chrome.Colors.OnSurfaceVariant
								if btn.Hovered() && canCreate {
									bg = c.view.Chrome.Colors.SurfaceContainerHigh
									fg = c.view.Chrome.Colors.OnSurface
								} else if !canCreate {
									fg = c.view.Chrome.Colors.OutlineVariant
								}
								label := "Start a new chat session"
								if snapshot.CreatingSession {
									label = "Starting a new chat session"
								}
								semantic.Button.Add(gtx.Ops)
								semantic.DescriptionOp(label).Add(gtx.Ops)
								return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
										return uikit.UniformInset(7).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return c.view.Chrome.ActionIcon(gtx, uikit.IconCompose, 16, fg)
										})
									})
								})
							}),
						)
					})
				}),
				// Full-width search bar: Search threads
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 2, Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return c.layoutSidebarSearch(gtx, rows)
					})
				}),
				// "Projects" section label
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Bottom: 6, Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return c.view.Chrome.Label(gtx, "Projects", uikit.TextLabelSmall, font.Medium, c.view.Chrome.Colors.OnSurfaceVariant, 1)
					})
				}),
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
		},
	)
}

func (c *Component) layoutSidebarSearch(gtx layout.Context, rows []Row) layout.Dimensions {
	for {
		evt, ok := gtx.Event(key.Filter{Focus: (&c.searchEditor), Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			(&c.searchEditor).SetText("")
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}
	for {
		evt, ok := gtx.Event(key.Filter{Focus: (&c.searchEditor), Name: key.NameReturn})
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			for _, r := range rows {
				if r.Kind == SessionRow {
					c.view.Actions.SelectSession(r.AgentID, r.SessionID)
					(&c.searchEditor).SetText("")
					gtx.Execute(key.FocusCmd{Tag: nil})
					break
				}
			}
		}
	}
	for {
		evt, ok := gtx.Event(key.Filter{Focus: (&c.searchEditor), Name: key.NameDownArrow})
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			for _, r := range rows {
				if r.Kind == SessionRow {
					widgetKey := r.SessionKey
					if widgetKey == "" {
						widgetKey = SessionWidgetKey(r.SessionID, r.AgentID)
					}
					btn := c.SessionButton(widgetKey)
					gtx.Execute(key.FocusCmd{Tag: btn})
					break
				}
			}
		}
	}
	if c.SearchClearButton().Clicked(gtx) {
		(&c.searchEditor).SetText("")
	}

	gtx.Constraints.Min.Y = gtx.Dp(28)
	isFocused := gtx.Focused((&c.searchEditor))
	borderColor := color.NRGBA{}
	borderWidth := 0
	iconColor := c.view.Chrome.Colors.OnSurfaceVariant
	if isFocused {
		borderColor = c.view.Chrome.Colors.Primary
		borderWidth = 1
		iconColor = c.view.Chrome.Colors.Primary
	}

	return c.view.Chrome.BorderSurface(gtx, uikit.ShapeMedium, c.view.Chrome.Colors.SurfaceContainerHigh, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return c.view.Chrome.ActionIcon(gtx, uikit.IconSearch, 16, iconColor)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(c.view.Chrome.Material, (&c.searchEditor), "Search threads…")
					ed.TextSize = uikit.TextBodySmall
					ed.Color = c.view.Chrome.Colors.OnSurface
					ed.HintColor = c.view.Chrome.Colors.OnSurfaceVariant
					return ed.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if (&c.searchEditor).Text() == "" {
						return layout.Dimensions{}
					}
					return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						semantic.Button.Add(gtx.Ops)
						semantic.DescriptionOp("Clear conversation search").Add(gtx.Ops)
						return c.layoutMiniIconButton(gtx, c.SearchClearButton(), "×", c.view.Chrome.Colors.OnSurfaceVariant)
					})
				}),
			)
		})
	})
}

type rowNavContext struct {
	rows  []Row
	index int
}

func (c *Component) layoutSidebarRow(gtx layout.Context, row Row, state desktopstate.State, nav ...rowNavContext) layout.Dimensions {
	if row.Kind == PinnedHeaderRow {
		button := c.PinnedButton()
		if button.Clicked(gtx) {
			c.TogglePinned()
		}
		chevronKind := uikit.IconChevronDown
		if c.PinnedCollapsed() {
			chevronKind = uikit.IconChevronRight
		}
		return uikit.Inset{Top: sidebarGroupTopInset, Bottom: sidebarGroupBottomInset, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(28)
				background := color.NRGBA{}
				foreground := c.view.Chrome.Colors.OnSurfaceVariant
				if button.Hovered() {
					background = c.view.Chrome.Colors.SurfaceContainerHigh
					foreground = c.view.Chrome.Colors.OnSurface
				}
				return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, background, func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.ActionIcon(gtx, chevronKind, RowGlyphSize, c.view.Chrome.Colors.OnSurfaceVariant)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.ActionIcon(gtx, uikit.IconStar, RowGlyphSize, c.view.Chrome.Colors.Primary)
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return c.view.Chrome.Label(gtx, "Pinned", uikit.TextLabelSmall, font.SemiBold, foreground, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if row.SessionCount > 0 {
									return c.layoutCountPill(gtx, row.SessionCount)
								}
								return layout.Dimensions{}
							}),
						)
					})
				})
			})
		})
	}

	if row.Kind == ProjectRow {
		button := c.ProjectButton(row.ProjectID)
		newBtn := c.ProjectNewButton(row.ProjectID)
		emptyNewBtn := c.ProjectEmptyNewButton(row.ProjectID)
		selected := state.ActiveProjectID == row.ProjectID && state.ActiveSessionID == ""
		isCollapsed := c.ProjectCollapsed(row.ProjectID)
		if newBtn.Clicked(gtx) || emptyNewBtn.Clicked(gtx) {
			if c.view.Actions.SelectProject != nil {
				c.view.Actions.SelectProject(row.ProjectID)
			}
			if c.view.Actions.NewSession != nil {
				c.view.Actions.NewSession()
			}
		} else if button.Clicked(gtx) {
			c.ToggleProject(row.ProjectID)
		}
		chevronKind := uikit.IconChevronDown
		if isCollapsed {
			chevronKind = uikit.IconChevronRight
		}
		isHovered := button != nil && button.Hovered()
		isFocused := button != nil && gtx.Focused(button)
		isNewHovered := newBtn != nil && newBtn.Hovered()
		showProjectActions := isHovered || isFocused || isNewHovered

		return uikit.Inset{Top: sidebarGroupTopInset, Bottom: sidebarGroupBottomInset, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = gtx.Dp(28)
				semantic.Button.Add(gtx.Ops)
				semantic.SelectedOp(selected).Add(gtx.Ops)
				semantic.DescriptionOp(fmt.Sprintf("Select workspace %s, %d conversations", row.Title, row.SessionCount)).Add(gtx.Ops)
				background := color.NRGBA{}
				foreground := c.view.Chrome.Colors.OnSurfaceVariant
				if selected {
					// A project selection stays subtler than the active thread:
					// one surface step above hover, never a filled pill.
					background = c.view.Chrome.Colors.SurfaceContainerHighest
					foreground = c.view.Chrome.Colors.OnSurface
				} else if button.Hovered() {
					background = c.view.Chrome.Colors.SurfaceContainerHigh
					foreground = c.view.Chrome.Colors.OnSurface
				}
				return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, background, func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.ActionIcon(gtx, chevronKind, RowGlyphSize, c.view.Chrome.Colors.OnSurfaceVariant)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.ActionIcon(gtx, uikit.IconFolder, RowGlyphSize, c.view.Chrome.Colors.OnSurfaceVariant)
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return c.view.Chrome.Label(gtx, row.Title, uikit.TextLabelSmall, font.SemiBold, foreground, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if !showProjectActions {
											return layout.Dimensions{}
										}
										return uikit.Inset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return c.layoutRowAction(gtx, newBtn, uikit.IconPlus, c.view.Chrome.Colors.OnSurfaceVariant, fmt.Sprintf("New session in %s", row.Title))
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if row.SessionCount > 0 {
											return c.layoutCountPill(gtx, row.SessionCount)
										}
										return layout.Dimensions{}
									}),
								)
							}),
						)
					})
				})
			})
			if isFocused {
				widget.Border{Color: c.view.Chrome.Colors.Primary, CornerRadius: uikit.ShapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: dims.Size}
				})
			}
			if !isCollapsed && row.SessionCount == 0 {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: dims.Size}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 4, Bottom: 6, Left: unit.Dp(RowIndent), Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.Label(gtx, "No conversations yet", uikit.TextLabelSmall, font.Normal, c.view.Chrome.Colors.OnSurfaceVariant, 1)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										semantic.Button.Add(gtx.Ops)
										semantic.DescriptionOp(fmt.Sprintf("Start first conversation in %s", row.Title)).Add(gtx.Ops)
										return c.layoutMiniButton(gtx, emptyNewBtn, "+ New", c.view.Chrome.Colors.Primary)
									})
								}),
							)
						})
					}),
				)
			}
			return dims
		})
	}

	widgetKey := row.SessionKey
	if widgetKey == "" {
		widgetKey = SessionWidgetKey(row.SessionID, row.AgentID)
	}
	button := c.SessionButton(widgetKey)
	pinBtn := c.PinButton(widgetKey)
	renameBtn := c.RenameButton(widgetKey)
	deleteBtn := c.DeleteButton(widgetKey)
	selected := state.ActiveSessionID == row.SessionID && (state.ActiveAgentID == "" || state.ActiveAgentID == row.AgentID)
	isRenaming := (&c.interaction).EditingSessionID == row.SessionID && (&c.interaction).EditingAgentID == row.AgentID
	isHovered := button != nil && button.Hovered()
	isFocused := button != nil && gtx.Focused(button)
	showActions := isHovered || isFocused

	if pinBtn.Clicked(gtx) {
		c.view.Actions.TogglePin(row.AgentID, row.SessionID)
	} else if renameBtn.Clicked(gtx) {
		(&c.interaction).EditingSessionID = row.SessionID
		(&c.interaction).EditingAgentID = row.AgentID
		c.RenameEditor().SetText(row.Title)
		c.RenameEditor().SetCaret(len(row.Title), len(row.Title))
		gtx.Execute(key.FocusCmd{Tag: c.RenameEditor()})
	} else if deleteBtn.Clicked(gtx) {
		(&c.interaction).DeletingSessionID = row.SessionID
		(&c.interaction).DeletingAgentID = row.AgentID
		(&c.interaction).DeletingSessionTitle = row.Title
	} else if button != nil && button.Clicked(gtx) {
		c.view.Actions.SelectSession(row.AgentID, row.SessionID)
	}

	if button != nil {
		for {
			evt, ok := gtx.Event(key.Filter{Focus: button, Name: key.NameDeleteBackward})
			if !ok {
				break
			}
			if e, ok := evt.(key.Event); ok && e.State == key.Press {
				(&c.interaction).DeletingSessionID = row.SessionID
				(&c.interaction).DeletingAgentID = row.AgentID
				(&c.interaction).DeletingSessionTitle = row.Title
			}
		}

		if len(nav) > 0 {
			allRows := nav[0].rows
			currentIndex := nav[0].index

			for {
				evt, ok := gtx.Event(key.Filter{Focus: button, Name: key.NameDownArrow})
				if !ok {
					break
				}
				if e, ok := evt.(key.Event); ok && e.State == key.Press {
					for i := currentIndex + 1; i < len(allRows); i++ {
						if allRows[i].Kind == SessionRow {
							targetKey := allRows[i].SessionKey
							if targetKey == "" {
								targetKey = SessionWidgetKey(allRows[i].SessionID, allRows[i].AgentID)
							}
							targetBtn := c.SessionButton(targetKey)
							gtx.Execute(key.FocusCmd{Tag: targetBtn})
							break
						}
					}
				}
			}

			for {
				evt, ok := gtx.Event(key.Filter{Focus: button, Name: key.NameUpArrow})
				if !ok {
					break
				}
				if e, ok := evt.(key.Event); ok && e.State == key.Press {
					foundPrev := false
					for i := currentIndex - 1; i >= 0; i-- {
						if allRows[i].Kind == SessionRow {
							targetKey := allRows[i].SessionKey
							if targetKey == "" {
								targetKey = SessionWidgetKey(allRows[i].SessionID, allRows[i].AgentID)
							}
							targetBtn := c.SessionButton(targetKey)
							gtx.Execute(key.FocusCmd{Tag: targetBtn})
							foundPrev = true
							break
						}
					}
					if !foundPrev {
						gtx.Execute(key.FocusCmd{Tag: &c.searchEditor})
					}
				}
			}
		}
	}

	if isRenaming {
		for {
			evt, ok := gtx.Event(key.Filter{Focus: c.RenameEditor(), Name: key.NameReturn})
			if !ok {
				break
			}
			if e, ok := evt.(key.Event); ok && e.State == key.Press {
				newTitle := strings.TrimSpace(c.RenameEditor().Text())
				(&c.interaction).EditingSessionID = ""
				c.view.Actions.RenameSession(row.AgentID, row.SessionID, newTitle)
			}
		}
		for {
			evt, ok := gtx.Event(key.Filter{Focus: c.RenameEditor(), Name: key.NameEscape})
			if !ok {
				break
			}
			if e, ok := evt.(key.Event); ok && e.State == key.Press {
				(&c.interaction).EditingSessionID = ""
			}
		}
		if c.RenameConfirmButton().Clicked(gtx) {
			newTitle := strings.TrimSpace(c.RenameEditor().Text())
			(&c.interaction).EditingSessionID = ""
			c.view.Actions.RenameSession(row.AgentID, row.SessionID, newTitle)
		}
		if c.RenameCancelButton().Clicked(gtx) {
			(&c.interaction).EditingSessionID = ""
		}
	}

	gtx.Constraints.Min.Y = gtx.Dp(sidebarSessionRowMinHeight)
	subtitle := SessionSubtitle(row, gtx.Now)
	statusLabel := StatusLabel(row.Status)

	return uikit.Inset{Top: 2, Bottom: 2, Left: unit.Dp(RowIndent), Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(sidebarSessionRowMinHeight)
			semantic.Button.Add(gtx.Ops)
			semantic.SelectedOp(selected).Add(gtx.Ops)
			description := row.Title
			if subtitle != "" {
				description += ", " + subtitle
			}
			if statusLabel != "" {
				description += ", " + statusLabel
			}
			semantic.DescriptionOp(description).Add(gtx.Ops)
			background := color.NRGBA{}
			// The active thread is a filled pill with on-primary text so selection
			// reads instantly; hover is one surface step above the resting row, so
			// the two never read the same.
			foreground := c.view.Chrome.Colors.OnSurface
			metaColor := c.view.Chrome.Colors.OnSurfaceVariant
			if selected {
				background = c.view.Chrome.Colors.PrimaryContainer
				foreground = c.view.Chrome.Colors.OnPrimaryContainer
				metaColor = c.view.Chrome.Colors.OnPrimaryContainer
			} else if isHovered {
				background = c.view.Chrome.Colors.SurfaceContainerHigh
			}
			return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeMedium, background, func(gtx layout.Context) layout.Dimensions {
				cardChildren := []layout.FlexChild{
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if isRenaming {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									ed := material.Editor(c.view.Chrome.Material, c.RenameEditor(), "Session title…")
									ed.TextSize = uikit.TextBodySmall
									ed.Color = foreground
									return ed.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return c.layoutMiniButton(gtx, c.RenameConfirmButton(), "✓", c.view.Chrome.Colors.Primary)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Left: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return c.layoutMiniButton(gtx, c.RenameCancelButton(), "✕", c.view.Chrome.Colors.OnSurfaceVariant)
									})
								}),
							)
						}
						weight := font.Normal
						if selected {
							weight = font.Medium
						}
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return c.view.Chrome.Label(gtx, row.Title, uikit.TextBodySmall, weight, foreground, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !showActions {
									if row.Pinned {
										return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return c.view.Chrome.ActionIcon(gtx, uikit.IconStar, RowGlyphSize, c.view.Chrome.Colors.Primary)
										})
									}
									return layout.Dimensions{}
								}
								// Hover/focus quick actions replace the old
								// kebab menu: pin, rename, and delete are one
								// click away without opening a popover.
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return uikit.Inset{Right: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											pinTint := c.view.Chrome.Colors.OnSurfaceVariant
											pinDesc := "Pin conversation"
											if row.Pinned {
												pinTint = c.view.Chrome.Colors.Primary
												pinDesc = "Unpin conversation"
											}
											return c.layoutRowAction(gtx, pinBtn, uikit.IconStar, pinTint, pinDesc)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return uikit.Inset{Right: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return c.layoutRowAction(gtx, renameBtn, uikit.IconCompose, c.view.Chrome.Colors.OnSurfaceVariant, "Rename conversation")
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return c.layoutRowAction(gtx, deleteBtn, uikit.IconTrash, c.view.Chrome.Colors.OnErrorContainer, "Delete conversation")
									}),
								)
							}),
						)
					}),
				}
				{
					// The metadata line always renders, even when both the
					// subtitle and status are empty, so a row never changes
					// height as a session moves between idle and running.
					cardChildren = append(cardChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = gtx.Dp(sidebarMetadataLineHeight)
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											badge := agentBadgeLabel(row)
											if badge == "" {
												return layout.Dimensions{}
											}
											return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return c.layoutAgentBadge(gtx, badge)
											})
										}),
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											if subtitle == "" {
												return layout.Dimensions{}
											}
											return c.view.Chrome.Label(gtx, subtitle, uikit.TextLabelSmall, font.Normal, metaColor, 1)
										}),
									)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if statusLabel == "" {
										return layout.Dimensions{}
									}
									return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return c.view.Chrome.TaskStatus(gtx, statusLabel, desktopstate.TaskStatus(row.Status))
									})
								}),
							)
						})
					}))
				}

				dims := uikit.Inset{Top: 6, Bottom: 6, Left: 12, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, cardChildren...)
				})

				if selected {
					barWidth := gtx.Dp(3)
					barInsetY := gtx.Dp(6)
					height := dims.Size.Y - (barInsetY * 2)
					if height > 0 {
						radius := barWidth / 2
						if radius < 1 {
							radius = 1
						}
						barRect := image.Rect(gtx.Dp(2), barInsetY, gtx.Dp(2)+barWidth, barInsetY+height)
						stack := clip.RRect{
							Rect: barRect,
							SE:   radius,
							SW:   radius,
							NW:   radius,
							NE:   radius,
						}.Push(gtx.Ops)
						paint.Fill(gtx.Ops, c.view.Chrome.Colors.Primary)
						stack.Pop()
					}
				}
				return dims
			})
		})
		if isFocused {
			widget.Border{Color: c.view.Chrome.Colors.Primary, CornerRadius: uikit.ShapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: dims.Size}
			})
		}
		return dims
	})
}

// layoutRowAction renders one hover-revealed session action as a compact icon
// button. The tint is kept on hover so destructive and pinned states stay
// legible instead of collapsing to a generic surface color.
func (c *Component) layoutRowAction(gtx layout.Context, button *widget.Clickable, icon uikit.Icon, tint color.NRGBA, description string) layout.Dimensions {
	background := color.NRGBA{}
	if button.Hovered() {
		background = c.view.Chrome.Colors.SurfaceContainerHighest
	}
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp(description).Add(gtx.Ops)
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Pt(gtx.Dp(22), gtx.Dp(22))
		gtx.Constraints.Max = image.Pt(gtx.Dp(22), gtx.Dp(22))
		return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return c.view.Chrome.ActionIcon(gtx, icon, 13, tint)
			})
		})
	})
}

// agentBadgeLabel returns the compact agent identity badge for a session row.
// The default agent has no badge so rows stay quiet unless ownership matters.
func agentBadgeLabel(row Row) string {
	agentID := strings.TrimSpace(row.AgentID)
	if agentID == "" || agentID == "protonman" || agentID == "universal" {
		return ""
	}
	switch strings.ToLower(agentID) {
	case "strength":
		return "STR"
	case "agility":
		return "AGI"
	case "intelligence":
		return "INT"
	}
	name := strings.TrimSpace(row.Subtitle)
	if name == "" {
		name = agentID
	}
	switch strings.ToLower(name) {
	case "strength", "strength agent":
		return "STR"
	case "agility", "agility agent":
		return "AGI"
	case "intelligence", "intelligence agent":
		return "INT"
	case "protonman", "universal":
		return ""
	}
	const maxBadgeLen = 14
	runes := []rune(name)
	if len(runes) > maxBadgeLen {
		return string(runes[:maxBadgeLen-1]) + "…"
	}
	return name
}

func (c *Component) layoutAgentBadge(gtx layout.Context, label string) layout.Dimensions {
	return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, c.view.Chrome.Colors.SecondaryContainer, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 1, Bottom: 1, Left: 5, Right: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return c.view.Chrome.Label(gtx, label, uikit.TextLabelSmall, font.Medium, c.view.Chrome.Colors.OnSecondaryContainer, 1)
		})
	})
}

func (c *Component) layoutMiniIconButton(gtx layout.Context, button *widget.Clickable, label string, normalColor color.NRGBA) layout.Dimensions {
	background := color.NRGBA{}
	foreground := normalColor
	if button.Hovered() {
		background = c.view.Chrome.Colors.SurfaceContainerHighest
		foreground = c.view.Chrome.Colors.OnSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Pt(gtx.Dp(22), gtx.Dp(22))
		gtx.Constraints.Max = image.Pt(gtx.Dp(22), gtx.Dp(22))
		return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if label == "✕" || label == "×" {
					return c.view.Chrome.ActionIcon(gtx, uikit.IconClose, 12, foreground)
				}
				return c.view.Chrome.Label(gtx, label, uikit.TextLabelSmall, font.Bold, foreground, 1)
			})
		})
	})
	return dims
}

func (c *Component) layoutCountPill(gtx layout.Context, count int) layout.Dimensions {
	return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeFull, c.view.Chrome.Colors.SurfaceContainerHigh, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return c.view.Chrome.Label(gtx, fmt.Sprintf("%d", count), uikit.TextLabelSmall, font.Medium, c.view.Chrome.Colors.OnSurfaceVariant, 1)
		})
	})
}

func (c *Component) layoutMiniButton(gtx layout.Context, button *widget.Clickable, label string, fg color.NRGBA) layout.Dimensions {
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(24)
		background := c.view.Chrome.Colors.SurfaceContainer
		if button.Hovered() {
			background = c.view.Chrome.Colors.SurfaceContainerHighest
		}
		return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return c.view.Chrome.Label(gtx, label, uikit.TextLabelSmall, font.Medium, fg, 1)
			})
		})
	})
	return dims
}

func (c *Component) clearDeleteTarget() {
	c.interaction.DeletingSessionID = ""
	c.interaction.DeletingAgentID = ""
	c.interaction.DeletingSessionTitle = ""
}

func (c *Component) layoutDeleteModal(gtx layout.Context) layout.Dimensions {
	if c.DeleteCancelButton().Clicked(gtx) || c.DeleteScrim().Clicked(gtx) {
		c.clearDeleteTarget()
	}

	for {
		evt, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			c.clearDeleteTarget()
		}
	}
	for {
		evt, ok := gtx.Event(key.Filter{Name: key.NameReturn})
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			id := c.interaction.DeletingSessionID
			agentID := c.interaction.DeletingAgentID
			c.clearDeleteTarget()
			if c.view.Actions.DeleteSession != nil && id != "" {
				c.view.Actions.DeleteSession(agentID, id)
			}
		}
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 160}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return c.DeleteScrim().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(380)
			gtx.Constraints.Max.X = gtx.Dp(440)
			return c.view.Chrome.BorderSurface(gtx, uikit.ShapeExtraLarge, c.view.Chrome.Colors.SurfaceContainerHigh, c.view.Chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
				return uikit.Inset{Top: 20, Bottom: 20, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return c.view.Chrome.Label(gtx, "Delete Session", uikit.TextTitleMedium, font.Bold, c.view.Chrome.Colors.OnSurface, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Top: 8, Bottom: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								msg := fmt.Sprintf("Are you sure you want to delete %q? This will delete all conversation turns and cannot be undone.", (&c.interaction).DeletingSessionTitle)
								return c.view.Chrome.Label(gtx, msg, uikit.TextBodyMedium, font.Normal, c.view.Chrome.Colors.OnSurfaceVariant, 3)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return c.view.Chrome.Button(gtx, c.DeleteCancelButton(), "Cancel", true, func() {
											c.clearDeleteTarget()
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.DangerButton(gtx, c.DeleteConfirmButton(), "Delete", true, func() {
										id := c.interaction.DeletingSessionID
										agentID := c.interaction.DeletingAgentID
										c.clearDeleteTarget()
										if c.view.Actions.DeleteSession != nil && id != "" {
											c.view.Actions.DeleteSession(agentID, id)
										}
									})
								}),
							)
						}),
					)
				})
			})
		}),
	)
}
