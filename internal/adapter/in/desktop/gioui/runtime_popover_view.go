//go:build desktop || desktop_gio

package gioui

import (
	"fmt"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

var popoverReasoningOptions = []struct {
	Level string
	Label string
	Desc  string
}{
	{Level: "auto", Label: "Auto", Desc: "Inherit default provider reasoning"},
	{Level: "low", Label: "Low", Desc: "Light reasoning for fast answers"},
	{Level: "medium", Label: "Medium", Desc: "Balanced thinking for coding"},
	{Level: "high", Label: "High", Desc: "Deep reasoning for architecture"},
	{Level: "max", Label: "Max", Desc: "Maximum reasoning budget"},
	{Level: "none", Label: "None", Desc: "Disable reasoning effort"},
}

func (s *shell) openModelPopover() {
	s.modelPopoverVisible = true
	s.reasoningPopoverVisible = false
	if s.onRefreshRuntime != nil {
		s.onRefreshRuntime()
	}
}

func (s *shell) openReasoningPopover() {
	s.reasoningPopoverVisible = true
	s.modelPopoverVisible = false
}

func (s *shell) closePopovers() {
	s.modelPopoverVisible = false
	s.reasoningPopoverVisible = false
}

func (s *shell) agentModelButton(name string) *widget.Clickable {
	btn, ok := s.agentModelButtons[name]
	if !ok {
		btn = new(widget.Clickable)
		if s.agentModelButtons == nil {
			s.agentModelButtons = make(map[string]*widget.Clickable)
		}
		s.agentModelButtons[name] = btn
	}
	return btn
}

func (s *shell) modelPresetButton(name string) *widget.Clickable {
	btn, ok := s.modelPresetButtons[name]
	if !ok {
		btn = new(widget.Clickable)
		if s.modelPresetButtons == nil {
			s.modelPresetButtons = make(map[string]*widget.Clickable)
		}
		s.modelPresetButtons[name] = btn
	}
	return btn
}

func (s *shell) popoverReasoningButton(level string) *widget.Clickable {
	btn, ok := s.popoverReasoningButtons[level]
	if !ok {
		btn = new(widget.Clickable)
		if s.popoverReasoningButtons == nil {
			s.popoverReasoningButtons = make(map[string]*widget.Clickable)
		}
		s.popoverReasoningButtons[level] = btn
	}
	return btn
}

func (s *shell) addRecentModel(provider, model, name string) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return
	}
	if name == "" {
		name = model
	}
	recents := make([]modelPresetRecord, 0, len(s.recentModels)+1)
	recents = append(recents, modelPresetRecord{Provider: provider, Model: model, Name: name})
	for _, r := range s.recentModels {
		if !(r.Provider == provider && r.Model == model) && len(recents) < 5 {
			recents = append(recents, r)
		}
	}
	s.recentModels = recents
}

func (s *shell) layoutModelPopover(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot, enabled bool) layout.Dimensions {
	if s.modelPopoverCloseButton.Clicked(gtx) {
		s.modelPopoverVisible = false
		return layout.Dimensions{}
	}

	maxHeight := gtx.Dp(320)
	gtx.Constraints.Max.Y = maxHeight

	currentModel := strings.TrimSpace(session.Runtime.Model)
	currentProvider := strings.TrimSpace(session.Runtime.Provider)
	agentID := strings.TrimSpace(session.AgentID)
	if agentID == "" {
		agentID = strings.TrimSpace(snapshot.ActiveAgentID)
	}
	if agentID == "" {
		agentID = controllerAgentID
	}
	agentName := agentDisplayName(snapshot.AgentProfiles, agentID)

	if currentModel == "" && snapshot.AgentDefaultModels != nil {
		currentModel = strings.TrimSpace(snapshot.AgentDefaultModels[agentID])
	}

	searchQuery := strings.ToLower(strings.TrimSpace(s.modelSearchEditor.Text()))
	rawModels := session.AvailableModels
	if len(rawModels) == 0 && snapshot.AgentAvailableModels != nil {
		rawModels = snapshot.AgentAvailableModels[agentID]
	}
	filteredModels := make([]string, 0, len(rawModels))
	for _, m := range rawModels {
		mTrimmed := strings.TrimSpace(m)
		if mTrimmed == "" {
			continue
		}
		if searchQuery == "" || strings.Contains(strings.ToLower(mTrimmed), searchQuery) {
			filteredModels = append(filteredModels, mTrimmed)
		}
	}

	return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, 5)

				// Header with agent display name and close button
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutModelPopoverHeader(gtx, agentName)
				}))

				// Divider
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 4, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutHorizontalDivider(gtx)
					})
				}))

				// Search input when the agent advertises models
				if len(rawModels) > 0 {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutModelSearchInput(gtx)
						})
					}))
				}

				// Content: empty state, no search matches, or scrollable model list
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if len(rawModels) == 0 {
						return s.layoutAgentModelsEmptyState(gtx, agentName, enabled)
					}
					if len(filteredModels) == 0 {
						return s.layoutModelSearchNoMatch(gtx, s.modelSearchEditor.Text())
					}
					return s.layoutAgentModelsList(gtx, filteredModels, currentModel, currentProvider, enabled)
				}))

				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

func (s *shell) layoutModelPopoverHeader(gtx layout.Context, agentName string) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "Select Model", textLabelLarge, font.SemiBold, s.theme.onSurface, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 2, Bottom: 2, Left: 7, Right: 7}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, agentName, textLabelSmall, font.Medium, s.theme.primary, 1)
					})
				})
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Alt+M · Esc", textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := &s.modelPopoverCloseButton
			semantic.Button.Add(gtx.Ops)
			semantic.DescriptionOp("Close model selector").Add(gtx.Ops)
			return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				fg := s.theme.onSurfaceVariant
				if btn.Hovered() {
					fg = s.theme.onSurface
				}
				return desktopInset{Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutActionIcon(gtx, iconClose, 12, fg)
				})
			})
		}),
	)
}

func (s *shell) layoutModelSearchInput(gtx layout.Context) layout.Dimensions {
	for {
		evt, ok := gtx.Event(key.Filter{Focus: &s.modelSearchEditor, Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			s.modelSearchEditor.SetText("")
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}
	if s.modelSearchClearBtn.Clicked(gtx) {
		s.modelSearchEditor.SetText("")
	}

	gtx.Constraints.Min.Y = gtx.Dp(28)
	isFocused := gtx.Focused(&s.modelSearchEditor)
	borderColor := color.NRGBA{}
	borderWidth := 0
	iconColor := s.theme.onSurfaceVariant
	if isFocused {
		borderColor = s.theme.primary
		borderWidth = 1
		iconColor = s.theme.primary
	}

	return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutActionIcon(gtx, iconSearch, 14, iconColor)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(s.theme.material, &s.modelSearchEditor, "Filter models…")
					ed.TextSize = textBodySmall
					ed.Color = s.theme.onSurface
					ed.HintColor = s.theme.onSurfaceVariant
					return ed.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if s.modelSearchEditor.Text() == "" {
						return layout.Dimensions{}
					}
					return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutMiniIconButton(gtx, &s.modelSearchClearBtn, "×", s.theme.onSurfaceVariant)
					})
				}),
			)
		})
	})
}

func (s *shell) layoutAgentModelsEmptyState(gtx layout.Context, agentName string, enabled bool) layout.Dimensions {
	if s.modelRefreshButton.Clicked(gtx) && enabled {
		if s.onRefreshRuntime != nil {
			s.onRefreshRuntime()
		}
	}

	return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 16, Bottom: 16, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "ⓘ", textTitleMedium, font.Normal, s.theme.onSurfaceVariant, 1)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					msg := fmt.Sprintf("%s manages models internally or does not advertise them over ACP.", agentName)
					return desktopInset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, msg, textBodySmall, font.Medium, s.theme.onSurface, 2)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "Configure models directly in the agent or press refresh to query.", textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 2)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := &s.modelRefreshButton
					semantic.Button.Add(gtx.Ops)
					semantic.EnabledOp(enabled).Add(gtx.Ops)
					bg := s.theme.surfaceContainerHighest
					fg := s.theme.onSurface
					if enabled && btn.Hovered() {
						bg = s.theme.primaryContainer
						fg = s.theme.onPrimaryContainer
					}
					if !enabled {
						gtx = gtx.Disabled()
					}
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedBorderSurface(gtx, shapeSmall, bg, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 5, Bottom: 5, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Refresh models (Alt+M)", textLabelSmall, font.SemiBold, fg, 1)
							})
						})
					})
				}),
			)
		})
	})
}

func (s *shell) layoutModelSearchNoMatch(gtx layout.Context, query string) layout.Dimensions {
	if s.modelSearchClearBtn.Clicked(gtx) {
		s.modelSearchEditor.SetText("")
	}

	return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 16, Bottom: 16, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					msg := fmt.Sprintf("No models match \"%s\"", query)
					return desktopInset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, msg, textBodySmall, font.Medium, s.theme.onSurfaceVariant, 1)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := &s.modelSearchClearBtn
					semantic.Button.Add(gtx.Ops)
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						fg := s.theme.primary
						if btn.Hovered() {
							fg = s.theme.onSurface
						}
						return s.layoutLabel(gtx, "Clear filter", textLabelSmall, font.SemiBold, fg, 1)
					})
				}),
			)
		})
	})
}

func (s *shell) layoutAgentModelsList(gtx layout.Context, models []string, currentModel, provider string, enabled bool) layout.Dimensions {
	s.modelList.Axis = layout.Vertical
	return s.modelList.Layout(gtx, len(models), func(gtx layout.Context, index int) layout.Dimensions {
		m := models[index]
		selected := m == currentModel
		return desktopInset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutModelListItem(gtx, m, provider, selected, enabled)
		})
	})
}

func (s *shell) layoutModelListItem(gtx layout.Context, modelID, provider string, selected, enabled bool) layout.Dimensions {
	btn := s.agentModelButton(modelID)
	if enabled && btn.Clicked(gtx) {
		if s.onSetRuntimeModel != nil {
			s.onSetRuntimeModel(provider, modelID)
		}
		s.modelPopoverVisible = false
	}

	semantic.Button.Add(gtx.Ops)
	semantic.EnabledOp(enabled).Add(gtx.Ops)
	semantic.SelectedOp(selected).Add(gtx.Ops)
	semantic.DescriptionOp("Select model " + modelID).Add(gtx.Ops)

	bg := s.theme.surfaceContainerLow
	fg := s.theme.onSurface
	if selected {
		bg = s.theme.primaryContainer
		fg = s.theme.onPrimaryContainer
	} else if enabled && btn.Hovered() {
		bg = s.theme.surfaceContainerHighest
	}
	if !enabled {
		gtx = gtx.Disabled()
	}

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		dims := s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if selected {
							return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "✓", textLabelMedium, font.Bold, fg, 1)
							})
						}
						return layout.Dimensions{}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, modelID, textBodySmall, font.Medium, fg, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if selected {
							return s.roundedSurface(gtx, shapeSmall, s.theme.primary, func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "Active", textLabelSmall, font.SemiBold, s.theme.onPrimary, 1)
								})
							})
						}
						return layout.Dimensions{}
					}),
				)
			})
		})
		if selected {
			widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: dims.Size}
			})
		}
		return dims
	})
}

func (s *shell) layoutReasoningPopover(gtx layout.Context, session desktopstate.SessionState, enabled bool) layout.Dimensions {
	if s.reasoningPopoverCloseBtn.Clicked(gtx) {
		s.reasoningPopoverVisible = false
		return layout.Dimensions{}
	}

	currentEffort := strings.TrimSpace(session.Runtime.Reasoning)
	if currentEffort == "" {
		currentEffort = "auto"
	}

	return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Reasoning Effort", textLabelLarge, font.SemiBold, s.theme.onSurface, 1)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "Alt+R · Esc", textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := &s.reasoningPopoverCloseBtn
								semantic.Button.Add(gtx.Ops)
								semantic.DescriptionOp("Close reasoning selector").Add(gtx.Ops)
								return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									fg := s.theme.onSurfaceVariant
									if btn.Hovered() {
										fg = s.theme.onSurface
									}
									return desktopInset{Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutActionIcon(gtx, iconClose, 12, fg)
									})
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 4, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutHorizontalDivider(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						rows := make([]layout.FlexChild, 0, len(popoverReasoningOptions))
						for _, opt := range popoverReasoningOptions {
							o := opt
							selected := o.Level == currentEffort
							btn := s.popoverReasoningButton(o.Level)
							if enabled && btn.Clicked(gtx) {
								s.onSetRuntimeReasoning(o.Level)
								s.reasoningPopoverVisible = false
							}
							rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									bg := s.theme.surfaceContainerLow
									fg := s.theme.onSurface
									descFg := s.theme.onSurfaceVariant
									if selected {
										bg = s.theme.primaryContainer
										fg = s.theme.onPrimaryContainer
										descFg = s.theme.onPrimaryContainer
									} else if enabled && btn.Hovered() {
										bg = s.theme.surfaceContainerHighest
									}
									return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										dims := s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 5, Bottom: 5, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														if selected {
															return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																return s.layoutLabel(gtx, "✓", textLabelSmall, font.Bold, fg, 1)
															})
														}
														return desktopInset{Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return layout.Spacer{}.Layout(gtx)
														})
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return s.layoutLabel(gtx, o.Label, textLabelMedium, font.SemiBold, fg, 1)
													}),
													layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
														return desktopInset{Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return s.layoutLabel(gtx, o.Desc, textLabelSmall, font.Normal, descFg, 1)
														})
													}),
												)
											})
										})
										if selected {
											widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return layout.Dimensions{Size: dims.Size}
											})
										}
										return dims
									})
								})
							}))
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
					}),
				)
			})
		})
	})
}
