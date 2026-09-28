//go:build desktop || desktop_gio

package gioui

import (
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"

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

func (s *shell) layoutModelPopover(gtx layout.Context, session desktopstate.SessionState, enabled bool) layout.Dimensions {
	if s.modelPopoverCloseButton.Clicked(gtx) {
		s.modelPopoverVisible = false
		return layout.Dimensions{}
	}

	maxHeight := gtx.Dp(260)
	gtx.Constraints.Max.Y = maxHeight

	currentModel := strings.TrimSpace(session.Runtime.Model)
	currentProvider := strings.TrimSpace(session.Runtime.Provider)
	if currentProvider == "" {
		currentProvider = "protonman"
	}

	return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutModelPopoverHeader(gtx, currentProvider)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 4, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutHorizontalDivider(gtx)
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						items := make([]layout.Widget, 0, 5)

						// 1. Models from Agent
						items = append(items, func(gtx layout.Context) layout.Dimensions {
							return s.layoutAgentModelsSection(gtx, session.AvailableModels, currentModel, currentProvider, enabled)
						})

						// 2. Curated Presets
						items = append(items, func(gtx layout.Context) layout.Dimensions {
							return s.layoutCuratedPresetsSection(gtx, currentModel, enabled)
						})

						// 3. Recent Models
						if len(s.recentModels) > 0 {
							items = append(items, func(gtx layout.Context) layout.Dimensions {
								return s.layoutRecentModelsSection(gtx, currentModel, enabled)
							})
						}

						// 4. Custom Model inputs
						items = append(items, func(gtx layout.Context) layout.Dimensions {
							return s.layoutCustomModelSection(gtx, currentProvider, enabled)
						})

						s.modelList.Axis = layout.Vertical
						return s.modelList.Layout(gtx, len(items), func(gtx layout.Context, index int) layout.Dimensions {
							return desktopInset{Bottom: 6}.Layout(gtx, items[index])
						})
					}),
				)
			})
		})
	})
}

func (s *shell) layoutModelPopoverHeader(gtx layout.Context, provider string) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "Select Model", textLabelLarge, font.SemiBold, s.theme.onSurface, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, provider, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
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
					return s.layoutLabel(gtx, "✕", textLabelMedium, font.Bold, fg, 1)
				})
			})
		}),
	)
}

func (s *shell) layoutAgentModelsSection(gtx layout.Context, models []string, currentModel, provider string, enabled bool) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			countStr := ""
			if len(models) > 0 {
				countStr = " (" + strings.TrimSpace(string(rune('0'+len(models)))) + ")"
				if len(models) >= 10 {
					countStr = " (10+)"
				}
			}
			return desktopInset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "FROM AGENT"+countStr, textLabelSmall, font.SemiBold, s.theme.primary, 1)
			})
		}),
	}

	if len(models) == 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, "Discovering models from agent… Press Alt+M to refresh", textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
				})
			})
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}

	// Layout agent models in rows of 2 or 3
	rowChildren := make([]layout.Widget, 0, (len(models)+1)/2)
	for i := 0; i < len(models); i += 2 {
		first := models[i]
		second := ""
		if i+1 < len(models) {
			second = models[i+1]
		}
		f := first
		sec := second
		rowChildren = append(rowChildren, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutModelPill(gtx, f, f, provider, f == currentModel, enabled, s.agentModelButton(f))
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if sec == "" {
						return layout.Spacer{}.Layout(gtx)
					}
					return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutModelPill(gtx, sec, sec, provider, sec == currentModel, enabled, s.agentModelButton(sec))
					})
				}),
			)
		})
	}

	for _, row := range rowChildren {
		r := row
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Bottom: 4}.Layout(gtx, r)
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutCuratedPresetsSection(gtx layout.Context, currentModel string, enabled bool) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 4, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "QUICK PRESETS", textLabelSmall, font.SemiBold, s.theme.onSurfaceVariant, 1)
			})
		}),
	}

	for _, preset := range curatedModelPresets {
		p := preset
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutModelPill(gtx, p.Name, p.Model, p.Provider, p.Model == currentModel, enabled, s.modelPresetButton(p.Model))
			})
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutRecentModelsSection(gtx layout.Context, currentModel string, enabled bool) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 4, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "RECENT", textLabelSmall, font.SemiBold, s.theme.onSurfaceVariant, 1)
			})
		}),
	}

	for _, item := range s.recentModels {
		it := item
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutModelPill(gtx, it.Name, it.Model, it.Provider, it.Model == currentModel, enabled, s.modelPresetButton("recent_"+it.Model))
			})
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutModelPill(gtx layout.Context, label, modelID, provider string, selected, enabled bool, btn *widget.Clickable) layout.Dimensions {
	if enabled && btn.Clicked(gtx) {
		s.onSetRuntimeModel(provider, modelID)
		s.addRecentModel(provider, modelID, label)
		s.modelPopoverVisible = false
	}

	semantic.Button.Add(gtx.Ops)
	semantic.EnabledOp(enabled).Add(gtx.Ops)
	semantic.SelectedOp(selected).Add(gtx.Ops)
	semantic.DescriptionOp("Select model " + label).Add(gtx.Ops)

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
			return desktopInset{Top: 5, Bottom: 5, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if selected {
							return desktopInset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "✓", textLabelSmall, font.Bold, fg, 1)
							})
						}
						return layout.Dimensions{}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, compactInspectorText(label, 30), textLabelSmall, font.Medium, fg, 1)
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

func (s *shell) layoutCustomModelSection(gtx layout.Context, defaultProvider string, enabled bool) layout.Dimensions {
	if s.popoverApplyModelButton.Clicked(gtx) && enabled {
		p := strings.TrimSpace(s.popoverProviderEditor.Text())
		if p == "" {
			p = defaultProvider
		}
		m := strings.TrimSpace(s.popoverModelEditor.Text())
		if p != "" && m != "" {
			s.onSetRuntimeModel(p, m)
			s.addRecentModel(p, m, m)
			s.modelPopoverVisible = false
		}
	}

	textColor := s.theme.onSurface
	textMaterial := op.Record(gtx.Ops)
	paint.ColorOp{Color: textColor}.Add(gtx.Ops)
	textCall := textMaterial.Stop()
	selectionMaterial := op.Record(gtx.Ops)
	paint.ColorOp{Color: s.theme.primaryContainer}.Add(gtx.Ops)
	selectionCall := selectionMaterial.Stop()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "CUSTOM MODEL", textLabelSmall, font.SemiBold, s.theme.onSurfaceVariant, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(0.4, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surface, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 4, Bottom: 4, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								if s.popoverProviderEditor.Text() == "" && !gtx.Focused(&s.popoverProviderEditor) {
									semantic.DescriptionOp("Provider").Add(gtx.Ops)
									return layout.Stack{}.Layout(gtx,
										layout.Stacked(func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, defaultProvider, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 1)
										}),
										layout.Expanded(func(gtx layout.Context) layout.Dimensions {
											return s.popoverProviderEditor.Layout(gtx, s.theme.material.Shaper, s.theme.textFont(font.Normal), textBodySmall, textCall, selectionCall)
										}),
									)
								}
								return s.popoverProviderEditor.Layout(gtx, s.theme.material.Shaper, s.theme.textFont(font.Normal), textBodySmall, textCall, selectionCall)
							})
						})
					})
				}),
				layout.Flexed(0.6, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surface, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 4, Bottom: 4, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								if s.popoverModelEditor.Text() == "" && !gtx.Focused(&s.popoverModelEditor) {
									return layout.Stack{}.Layout(gtx,
										layout.Stacked(func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "model-id…", textBodySmall, font.Normal, s.theme.onSurfaceVariant, 1)
										}),
										layout.Expanded(func(gtx layout.Context) layout.Dimensions {
											return s.popoverModelEditor.Layout(gtx, s.theme.material.Shaper, s.theme.textFont(font.Normal), textBodySmall, textCall, selectionCall)
										}),
									)
								}
								return s.popoverModelEditor.Layout(gtx, s.theme.material.Shaper, s.theme.textFont(font.Normal), textBodySmall, textCall, selectionCall)
							})
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					canApply := strings.TrimSpace(s.popoverModelEditor.Text()) != ""
					btn := &s.popoverApplyModelButton
					bg := s.theme.primary
					fg := s.theme.onPrimary
					if !canApply || !enabled {
						bg = s.theme.surfaceContainerLow
						fg = s.theme.onSurfaceVariant
					} else if btn.Hovered() {
						bg = s.theme.primaryContainer
						fg = s.theme.onPrimaryContainer
					}
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 5, Bottom: 5, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Apply", textLabelSmall, font.SemiBold, fg, 1)
							})
						})
					})
				}),
			)
		}),
	)
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
										return s.layoutLabel(gtx, "✕", textLabelMedium, font.Bold, fg, 1)
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
