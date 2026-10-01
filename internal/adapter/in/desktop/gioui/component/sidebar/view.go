//go:build desktop || desktop_gio

package sidebar

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
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

func taskStatusFromLabel(label string) desktopstate.TaskStatus {
	return desktopstate.TaskStatus(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(label)), " ", "_"))
}

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
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.layoutSidebarHeader(gtx, snapshot)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			allRows := c.Rows(Model{State: snapshot.State, AgentProfiles: snapshot.AgentProfiles, Pinned: snapshot.PinnedSessions, CustomTitles: snapshot.CustomTitles, FilterMode: snapshot.FilterMode, Revision: snapshot.Revision})
			rows := c.DisplayRows(allRows, c.searchEditor.Text(), snapshot.FilterMode, snapshot.Revision)
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
					emptyText = "No pinned conversations. Click ⋮ on a session to pin it."
					hasFilter = true
				}
				if c.ClearFilterButton().Clicked(gtx) {
					(&c.searchEditor).SetText("")
					if c.view.Actions.SetFilterMode != nil {
						c.view.Actions.SetFilterMode("all")
					}
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
						}
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
					},
				)
			}
			return (&c.list).Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
				return c.layoutSidebarRow(gtx, rows[index], snapshot.State)
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
							if c.CommunityButton().Hovered() {
								bg = c.view.Chrome.Colors.SurfaceContainerHigh
								fg = c.view.Chrome.Colors.OnSurface
							}
							semantic.Button.Add(btnGtx.Ops)
							semantic.DescriptionOp("Open Protonman Community").Add(btnGtx.Ops)
							return c.CommunityButton().Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
								return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
									return uikit.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return c.view.Chrome.ActionIcon(gtx, uikit.IconDiscord, 16, fg)
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

func (c *Component) layoutSidebarHeader(gtx layout.Context, snapshot Snapshot) layout.Dimensions {
	return uikit.Inset{Top: 14, Bottom: 4, Left: 12, Right: 12}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			newLabel := "New chat"
			if snapshot.CreatingSession {
				newLabel = "Starting…"
			}
			canCreate := snapshot.Connection == "connected" && !snapshot.CreatingSession

			items := []layout.FlexChild{
				// Brand Header: [P] Protonman
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Bottom: 14, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
					})
				}),
				// "New chat" dedicated action row
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := c.NewSessionButton()
					if btn.Clicked(gtx) && canCreate && c.view.Actions.NewSession != nil {
						c.view.Actions.NewSession()
					}
					bg := color.NRGBA{}
					fg := c.view.Chrome.Colors.OnSurface
					if btn.Hovered() && canCreate {
						bg = c.view.Chrome.Colors.SurfaceContainerHigh
					}
					semantic.Button.Add(gtx.Ops)
					semantic.DescriptionOp("Start a new chat session").Add(gtx.Ops)
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Top: 7, Bottom: 7, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return uikit.Inset{Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return c.view.Chrome.ActionIcon(gtx, uikit.IconCompose, 16, fg)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return c.view.Chrome.Label(gtx, newLabel, uikit.TextBodyMedium, font.Medium, fg, 1)
									}),
								)
							})
						})
					})
				}),
				// Full-width search bar: Search threads
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 6, Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return c.layoutSidebarSearch(gtx)
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

func (c *Component) layoutSidebarSearch(gtx layout.Context) layout.Dimensions {
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

func (c *Component) sidebarDisplayRows(rows []Row, filterMode string, sourceRevisions ...uint64) []Row {
	sourceRevision := uint64(0)
	if len(sourceRevisions) > 0 {
		sourceRevision = sourceRevisions[0]
	}
	return c.DisplayRows(rows, (&c.searchEditor).Text(), filterMode, sourceRevision)
}

func (c *Component) layoutSidebarRow(gtx layout.Context, row Row, state desktopstate.State) layout.Dimensions {
	if row.Kind == PinnedHeaderRow {
		button := c.PinnedButton()
		if button.Clicked(gtx) {
			c.TogglePinned()
		}
		gtx.Constraints.Min.Y = gtx.Dp(32)
		chevronKind := uikit.IconChevronDown
		if c.PinnedCollapsed() {
			chevronKind = uikit.IconChevronRight
		}
		dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
		return uikit.Inset{Top: sidebarGroupTopInset, Bottom: sidebarGroupBottomInset, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}

	if row.Kind == ProjectRow {
		button := c.ProjectButton(row.ProjectID)
		selected := state.ActiveProjectID == row.ProjectID && state.ActiveSessionID == ""
		isCollapsed := c.ProjectCollapsed(row.ProjectID)
		if button.Clicked(gtx) {
			c.ToggleProject(row.ProjectID)
			c.view.Actions.SelectProject(row.ProjectID)
		}
		gtx.Constraints.Min.Y = gtx.Dp(32)
		chevronKind := uikit.IconChevronDown
		if isCollapsed {
			chevronKind = uikit.IconChevronRight
		}
		dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(28)
			semantic.Button.Add(gtx.Ops)
			semantic.SelectedOp(selected).Add(gtx.Ops)
			semantic.DescriptionOp(fmt.Sprintf("Select workspace %s, %d conversations", row.Title, row.SessionCount)).Add(gtx.Ops)
			background := color.NRGBA{}
			foreground := c.view.Chrome.Colors.OnSurfaceVariant
			if selected {
				background = c.view.Chrome.Colors.SurfaceContainerHigh
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
							if row.SessionCount > 0 {
								return c.layoutCountPill(gtx, row.SessionCount)
							}
							return layout.Dimensions{}
						}),
					)
				})
			})
		})
		if gtx.Focused(button) {
			widget.Border{Color: c.view.Chrome.Colors.Primary, CornerRadius: uikit.ShapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: dims.Size}
			})
		}
		return uikit.Inset{Top: sidebarGroupTopInset, Bottom: sidebarGroupBottomInset, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}

	widgetKey := row.SessionKey
	if widgetKey == "" {
		widgetKey = SessionWidgetKey(row.SessionID, row.AgentID)
	}
	button := c.SessionButton(widgetKey)
	pinBtn := c.PinButton(row.SessionID, row.AgentID)
	renameBtn := c.RenameButton(row.SessionID, row.AgentID)
	deleteBtn := c.DeleteButton(row.SessionID, row.AgentID)
	menuBtn := c.MenuButton(row.SessionID, row.AgentID)
	selected := state.ActiveSessionID == row.SessionID && (state.ActiveAgentID == "" || state.ActiveAgentID == row.AgentID)
	isRenaming := (&c.interaction).EditingSessionID == row.SessionID && (&c.interaction).EditingAgentID == row.AgentID
	menuOpen := (&c.interaction).MenuSessionID == row.SessionID && (&c.interaction).MenuAgentID == row.AgentID
	isHovered := button != nil && button.Hovered()
	isFocused := button != nil && gtx.Focused(button)
	showActions := isHovered || selected || menuOpen || isFocused

	if button != nil && button.Clicked(gtx) {
		c.view.Actions.SelectSession(row.AgentID, row.SessionID)
	}

	if pinBtn.Clicked(gtx) {
		c.view.Actions.TogglePin(row.AgentID, row.SessionID)
	}
	if renameBtn.Clicked(gtx) {
		(&c.interaction).EditingSessionID = row.SessionID
		(&c.interaction).EditingAgentID = row.AgentID
		c.RenameEditor().SetText(row.Title)
	}
	if deleteBtn.Clicked(gtx) {
		(&c.interaction).DeletingSessionID = row.SessionID
		(&c.interaction).DeletingAgentID = row.AgentID
		(&c.interaction).DeletingSessionTitle = row.Title
	}

	if isRenaming {
		for {
			evt, ok := gtx.Event(key.Filter{Focus: c.RenameEditor(), Name: key.NameReturn})
			if !ok {
				break
			}
			if e, ok := evt.(key.Event); ok && e.State == key.Press {
				newTitle := c.RenameEditor().Text()
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
			newTitle := c.RenameEditor().Text()
			(&c.interaction).EditingSessionID = ""
			c.view.Actions.RenameSession(row.AgentID, row.SessionID, newTitle)
		}
		if c.RenameCancelButton().Clicked(gtx) {
			(&c.interaction).EditingSessionID = ""
		}
	}

	if menuBtn.Clicked(gtx) {
		if (&c.interaction).MenuSessionID == row.SessionID && (&c.interaction).MenuAgentID == row.AgentID {
			(&c.interaction).MenuSessionID = ""
			(&c.interaction).MenuAgentID = ""
		} else {
			(&c.interaction).MenuSessionID = row.SessionID
			(&c.interaction).MenuAgentID = row.AgentID
		}
	}

	gtx.Constraints.Min.Y = gtx.Dp(sidebarSessionRowMinHeight)
	subtitle := SessionSubtitle(row, gtx.Now)
	statusLabel := StatusLabel(row.Status)

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
		// The title is the row's primary label, so it uses the primary text
		// color; only metadata (subtitle, status) stays muted. Selected is one
		// surface step brighter than hover so the two never read the same.
		foreground := c.view.Chrome.Colors.OnSurface
		if selected {
			background = c.view.Chrome.Colors.SurfaceContainerHighest
		} else if isHovered {
			background = c.view.Chrome.Colors.SurfaceContainerHigh
		}
		return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeMedium, background, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Top: 6, Bottom: 6, Left: 12, Right: 8}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
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
									if row.Pinned && !showActions {
										return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return c.view.Chrome.ActionIcon(gtx, uikit.IconStar, RowGlyphSize, c.view.Chrome.Colors.Primary)
										})
									}
									if !showActions {
										return layout.Dimensions{}
									}
									var items []layout.FlexChild
									if row.Pinned {
										items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return c.view.Chrome.ActionIcon(gtx, uikit.IconStar, RowGlyphSize, c.view.Chrome.Colors.Primary)
											})
										}))
									}
									items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return c.layoutMiniMenuButton(gtx, menuBtn, "⋮")
									}))
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, items...)
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
										if subtitle == "" {
											return layout.Dimensions{}
										}
										return c.view.Chrome.Label(gtx, subtitle, uikit.TextLabelSmall, font.Normal, c.view.Chrome.Colors.OnSurfaceVariant, 1)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if statusLabel == "" {
											return layout.Dimensions{}
										}
										return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return c.view.Chrome.TaskStatus(gtx, statusLabel, taskStatusFromLabel(row.Status))
										})
									}),
								)
							})
						}))
					}
					if menuOpen {
						cardChildren = append(cardChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Top: 6, Bottom: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return c.layoutSessionMenuPopover(gtx, row)
							})
						}))
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, cardChildren...)
				},
			)
		})
	})
	if isFocused {
		widget.Border{Color: c.view.Chrome.Colors.Primary, CornerRadius: uikit.ShapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	leftPad := unit.Dp(RowIndent)
	if row.Pinned {
		leftPad = unit.Dp(8)
	}
	return uikit.Inset{Top: 2, Bottom: 2, Left: leftPad, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: dims.Size}
	})
}

func (c *Component) sessionMenuButton(sessionID string, agentIDs ...string) *widget.Clickable {
	return c.MenuButton(sessionID, agentIDs...)
}

func (c *Component) sessionPinButton(sessionID string, agentIDs ...string) *widget.Clickable {
	return c.PinButton(sessionID, agentIDs...)
}

func (c *Component) sessionQuickRenameButton(sessionID string, agentIDs ...string) *widget.Clickable {
	return c.RenameButton(sessionID, agentIDs...)
}

func (c *Component) sessionQuickDeleteButton(sessionID string, agentIDs ...string) *widget.Clickable {
	return c.DeleteButton(sessionID, agentIDs...)
}

func (c *Component) layoutMiniMenuButton(gtx layout.Context, button *widget.Clickable, label string) layout.Dimensions {
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("Open conversation actions").Add(gtx.Ops)
	background := color.NRGBA{}
	foreground := c.view.Chrome.Colors.OnSurfaceVariant
	if button.Hovered() {
		background = c.view.Chrome.Colors.SurfaceContainerHighest
		foreground = c.view.Chrome.Colors.OnSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Pt(gtx.Dp(22), gtx.Dp(22))
		gtx.Constraints.Max = image.Pt(gtx.Dp(22), gtx.Dp(22))
		return c.view.Chrome.RoundedSurface(gtx, uikit.ShapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if label == "⋮" {
					return c.view.Chrome.ActionIcon(gtx, uikit.IconKebab, 14, foreground)
				}
				return c.view.Chrome.Label(gtx, label, uikit.TextLabelMedium, font.Bold, foreground, 1)
			})
		})
	})
	return dims
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

func (c *Component) layoutSessionMenuPopover(gtx layout.Context, row Row) layout.Dimensions {
	pinLabel := "Pin"
	if row.Pinned {
		pinLabel = "Unpin"
	}

	if c.MenuPinButton().Clicked(gtx) {
		(&c.interaction).MenuSessionID = ""
		c.view.Actions.TogglePin(row.AgentID, row.SessionID)
	}
	if c.MenuRenameButton().Clicked(gtx) {
		(&c.interaction).MenuSessionID = ""
		(&c.interaction).EditingSessionID = row.SessionID
		(&c.interaction).EditingAgentID = row.AgentID
		c.RenameEditor().SetText(row.Title)
	}
	if c.MenuDeleteButton().Clicked(gtx) {
		(&c.interaction).MenuSessionID = ""
		(&c.interaction).DeletingSessionID = row.SessionID
		(&c.interaction).DeletingAgentID = row.AgentID
		(&c.interaction).DeletingSessionTitle = row.Title
	}

	return c.view.Chrome.BorderSurface(gtx, uikit.ShapeSmall, c.view.Chrome.Colors.SurfaceContainerHighest, c.view.Chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 4, Bottom: 4, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.layoutMiniButton(gtx, c.MenuPinButton(), pinLabel, c.view.Chrome.Colors.Primary)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.layoutMiniButton(gtx, c.MenuRenameButton(), "Rename", c.view.Chrome.Colors.OnSurface)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.layoutMiniButton(gtx, c.MenuDeleteButton(), "Delete", c.view.Chrome.Colors.OnErrorContainer)
				}),
			)
		})
	})
}

func (c *Component) layoutDeleteModal(gtx layout.Context) layout.Dimensions {
	if c.DeleteCancelButton().Clicked(gtx) || c.DeleteScrim().Clicked(gtx) {
		(&c.interaction).DeletingSessionID = ""
		(&c.interaction).DeletingSessionTitle = ""
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
											(&c.interaction).DeletingSessionID = ""
											(&c.interaction).DeletingSessionTitle = ""
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return c.view.Chrome.DangerButton(gtx, c.DeleteConfirmButton(), "Delete", true, func() {
										id := (&c.interaction).DeletingSessionID
										agentID := (&c.interaction).DeletingAgentID
										(&c.interaction).DeletingSessionID = ""
										(&c.interaction).DeletingAgentID = ""
										(&c.interaction).DeletingSessionTitle = ""
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

func (c *Component) layoutSidebarTaskStatus(gtx layout.Context, label string, status desktopstate.TaskStatus) layout.Dimensions {
	return c.view.Chrome.TaskStatus(gtx, label, status)
}
