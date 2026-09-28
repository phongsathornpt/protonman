//go:build desktop || desktop_gio

package gioui

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

var themeChoices = []struct {
	Mode  string
	Label string
	Desc  string
}{
	{Mode: "system", Label: "System Default", Desc: "Match your OS appearance automatically"},
	{Mode: "dark", Label: "macOS Dark", Desc: "Deep near-black dark theme"},
	{Mode: "light", Label: "macOS Light", Desc: "Clean high-contrast light theme"},
	{Mode: "slate-dark", Label: "Slate Dark", Desc: "GitHub-style slate dark palette"},
	{Mode: "slate-light", Label: "Slate Light", Desc: "Soft slate light palette"},
}

func (s *shell) openSettingsModal() {
	s.settingsModalOpen = true
}

func (s *shell) closeSettingsModal() {
	s.settingsModalOpen = false
}

func (s *shell) layoutSettingsModal(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	if s.settingsModalCloseBtn.Clicked(gtx) || s.settingsModalScrim.Clicked(gtx) {
		s.closeSettingsModal()
	}

	for {
		event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if ev, ok := event.(key.Event); ok && ev.State == key.Press {
			s.closeSettingsModal()
		}
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 160}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return s.settingsModalScrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			width := min(gtx.Dp(580), gtx.Constraints.Max.X-gtx.Dp(32))
			maxHeight := gtx.Constraints.Max.Y - gtx.Dp(60)
			gtx.Constraints.Min.X = width
			gtx.Constraints.Max.X = width
			gtx.Constraints.Max.Y = maxHeight

			return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 16, Bottom: 16, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						// Header: Title + Close Button
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutActionIcon(gtx, iconSettings, 18, s.theme.primary)
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "Settings", textTitleLarge, font.Bold, s.theme.onSurface, 1)
										}),
									)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutMiniIconButton(gtx, &s.settingsModalCloseBtn, "✕", s.theme.onSurfaceVariant)
								}),
							)
						}),
						// Tab Bar
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 14, Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutSettingsTabBar(gtx)
							})
						}),
						// Tab Content (Scrollable list if long)
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return s.settingsModalList.Layout(gtx, 1, func(gtx layout.Context, index int) layout.Dimensions {
								switch s.settingsActiveTab {
								case 1:
									return s.layoutMCPIntegrationsPanel(gtx, snapshot)
								case 2:
									return s.layoutAgentProfilesPanel(gtx, snapshot)
								default:
									return s.layoutSettingsGeneralTab(gtx, snapshot)
								}
							})
						}),
					)
				})
			})
		}),
	)
}

func (s *shell) layoutSettingsTabBar(gtx layout.Context) layout.Dimensions {
	tabs := []string{"General", "MCP Integrations", "ACP Agents"}
	return s.roundedSurface(gtx, shapeMedium, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
		return desktopUniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(tabs))
			for i, tab := range tabs {
				tabIdx := i
				tabLabel := tab
				active := s.settingsActiveTab == tabIdx
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					btn := &s.settingsTabButtons[tabIdx]
					if btn.Clicked(gtx) {
						s.settingsActiveTab = tabIdx
					}
					bg := color.NRGBA{}
					fg := s.theme.onSurfaceVariant
					if active {
						bg = s.theme.surface
						fg = s.theme.onSurface
					} else if btn.Hovered() {
						bg = s.theme.surfaceContainerHigh
						fg = s.theme.onSurface
					}
					gtx.Constraints.Min.Y = gtx.Dp(30)
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 5, Bottom: 5, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									weight := font.Medium
									if active {
										weight = font.SemiBold
									}
									return s.layoutLabel(gtx, tabLabel, textLabelMedium, weight, fg, 1)
								}))
							})
						})
					})
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}

func (s *shell) layoutSettingsGeneralTab(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	currentTheme := "system"
	if snapshot.Theme != "" {
		currentTheme = snapshot.Theme
	}

	systemDark := isSystemDarkMode()
	systemDesc := "Automatically match your OS appearance (currently Light)"
	if systemDark {
		systemDesc = "Automatically match your OS appearance (currently Dark)"
	}

	children := []layout.FlexChild{
		// Section: Appearance
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "APPEARANCE", textLabelSmall, font.Bold, s.theme.onSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 4, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Select the visual color scheme for the application chrome and components.", textBodySmall, font.Normal, s.theme.onSurfaceVariant, 2)
			})
		}),
		// System Default (Featured full-width option)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutThemeChoiceButton(gtx, themeChoices[0].Mode, themeChoices[0].Label, systemDesc, currentTheme == themeChoices[0].Mode)
			})
		}),
		// Explicit theme choices (2x2 grid)
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 4, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutThemeChoiceButton(gtx, themeChoices[1].Mode, themeChoices[1].Label, themeChoices[1].Desc, currentTheme == themeChoices[1].Mode)
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Left: 4, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutThemeChoiceButton(gtx, themeChoices[2].Mode, themeChoices[2].Label, themeChoices[2].Desc, currentTheme == themeChoices[2].Mode)
							})
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 4, Top: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutThemeChoiceButton(gtx, themeChoices[3].Mode, themeChoices[3].Label, themeChoices[3].Desc, currentTheme == themeChoices[3].Mode)
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Left: 4, Top: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutThemeChoiceButton(gtx, themeChoices[4].Mode, themeChoices[4].Label, themeChoices[4].Desc, currentTheme == themeChoices[4].Mode)
							})
						}),
					)
				}),
			)
		}),
		// Divider
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 16, Bottom: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutHorizontalDivider(gtx)
			})
		}),
		// Section: About
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "ABOUT PROTONMAN DESKTOP", textLabelSmall, font.Bold, s.theme.onSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Protonman Autonomous Coding Agent", textBodyMedium, font.SemiBold, s.theme.onSurface, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "Native Gio frontend driving the Protonman runtime over Agent Client Protocol (ACP) JSON-RPC stdio. Preserves clean architecture boundaries, fail-closed permission enforcement, and workspace confinement.", textBodySmall, font.Normal, s.theme.onSurfaceVariant, 4)
		}),
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutThemeChoiceButton(gtx layout.Context, mode, label, desc string, selected bool) layout.Dimensions {
	if s.settingsThemeButtons == nil {
		s.settingsThemeButtons = make(map[string]*widget.Clickable)
	}
	btn, ok := s.settingsThemeButtons[mode]
	if !ok {
		btn = new(widget.Clickable)
		s.settingsThemeButtons[mode] = btn
	}

	if btn.Clicked(gtx) && s.onSetTheme != nil {
		s.onSetTheme(mode)
	}

	bg := s.theme.surface
	borderColor := s.theme.outlineVariant
	borderWidth := 1
	if selected {
		bg = s.theme.primaryContainer
		borderColor = s.theme.primary
		borderWidth = 2
	} else if btn.Hovered() {
		bg = s.theme.surfaceContainerHigh
	}

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeMedium, bg, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						textColor := s.theme.onSurface
						if selected {
							textColor = s.theme.onPrimaryContainer
						}
						return s.layoutLabel(gtx, label, textLabelLarge, font.SemiBold, textColor, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						textColor := s.theme.onSurfaceVariant
						if selected {
							textColor = s.theme.onPrimaryContainer
						}
						return desktopInset{Top: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, desc, textLabelSmall, font.Normal, textColor, 2)
						})
					}),
				)
			})
		})
	})
}
