//go:build desktop || desktop_gio

package gioui

import (
	"fmt"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

var defaultPresetCatalog = []desktopstate.ProviderState{
	{ID: "opencode", Name: "OpenCode (Free)", Protocol: "openai", RequiresKey: false, IsFree: true},
	{ID: "opencode-zen", Name: "OpenCode Zen", Protocol: "openai", RequiresKey: true},
	{ID: "opencode-go", Name: "OpenCode Go", Protocol: "openai", RequiresKey: true},
	{ID: "protonman", Name: "Protonman", Protocol: "openai", RequiresKey: true},
	{ID: "ollama", Name: "Ollama (Local)", Protocol: "openai", RequiresKey: false, IsFree: true},
	{ID: "openai", Name: "OpenAI Official", Protocol: "openai", RequiresKey: true},
	{ID: "anthropic", Name: "Anthropic", Protocol: "anthropic", RequiresKey: true},
}

func (s *shell) providerCardButton(id string) *widget.Clickable {
	btn, ok := s.providerCardButtons[id]
	if !ok {
		btn = new(widget.Clickable)
		if s.providerCardButtons == nil {
			s.providerCardButtons = make(map[string]*widget.Clickable)
		}
		s.providerCardButtons[id] = btn
	}
	return btn
}

func (s *shell) providerModelSelectButton(id string) *widget.Clickable {
	btn, ok := s.providerModelSelectBtns[id]
	if !ok {
		btn = new(widget.Clickable)
		if s.providerModelSelectBtns == nil {
			s.providerModelSelectBtns = make(map[string]*widget.Clickable)
		}
		s.providerModelSelectBtns[id] = btn
	}
	return btn
}

func (s *shell) layoutProvidersPanel(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	providers := snapshot.Providers
	if len(providers) == 0 {
		providers = defaultPresetCatalog
	}
	enabled := !snapshot.ProviderUpdating

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPanelTitle(gtx, "Model Providers")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			summary := "Configure API keys, endpoints, and default models for LLM providers."
			return desktopInset{Top: 2, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, summary, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 2)
			})
		}),
	}

	for _, p := range providers {
		item := p
		isSelected := s.providerFormVisible && s.providerFormSelectedID == item.ID
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutProviderCard(gtx, item, isSelected, enabled)
		}))
	}

	if !s.providerFormVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				btn := &s.providerAddCustomBtn
				if enabled && btn.Clicked(gtx) {
					s.openProviderForm("", "", "https://api.example.com/v1", "openai", "")
				}
				return s.layoutButton(gtx, btn, "+ Add Custom Provider / Proxy", enabled, nil)
			})
		}))
	} else {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutProviderForm(gtx, snapshot, enabled)
			})
		}))
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) openProviderForm(id, name, endpoint, protocol, defaultModel string) {
	s.providerFormVisible = true
	s.providerFormSelectedID = id
	s.providerNameEditor.SetText(name)
	s.providerEndpointEditor.SetText(endpoint)
	s.providerAPIKeyEditor.SetText("")
	s.providerSelectedType = protocol
	if s.providerSelectedType == "" {
		s.providerSelectedType = "openai"
	}
	s.providerShowKey = false
	s.providerTestStatus = ""
	s.providerTestError = ""
	s.providerDiscoveredModels = nil
	s.providerSelectedModel = defaultModel
	s.providerModelsDropdownOpen = false
}

func (s *shell) layoutProviderCard(gtx layout.Context, p desktopstate.ProviderState, isSelected, enabled bool) layout.Dimensions {
	btn := s.providerCardButton(p.ID)
	if enabled && btn.Clicked(gtx) {
		if isSelected {
			s.providerFormVisible = false
			s.providerFormSelectedID = ""
		} else {
			s.openProviderForm(p.ID, p.Name, p.BaseURL, p.Protocol, p.DefaultModel)
		}
	}

	semantic.Button.Add(gtx.Ops)
	semantic.EnabledOp(enabled).Add(gtx.Ops)
	semantic.DescriptionOp("Configure provider " + p.Name).Add(gtx.Ops)

	bg := s.theme.surfaceContainerLow
	border := s.theme.outlineVariant
	if isSelected {
		bg = s.theme.surfaceContainerHigh
		border = s.theme.primary
	} else if enabled && btn.Hovered() {
		bg = s.theme.surfaceContainerHighest
	}

	return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.roundedBorderSurface(gtx, shapeSmall, bg, border, 1, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, p.Name, textBodyMedium, font.SemiBold, s.theme.onSurface, 1)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if p.IsActive {
												return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return s.roundedSurface(gtx, shapeSmall, s.theme.primary, func(gtx layout.Context) layout.Dimensions {
														return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return s.layoutLabel(gtx, "Active Default", textLabelSmall, font.SemiBold, s.theme.onPrimary, 1)
														})
													})
												})
											}
											return layout.Dimensions{}
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if p.IsFree {
												return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return s.roundedSurface(gtx, shapeSmall, s.theme.successContainer, func(gtx layout.Context) layout.Dimensions {
														return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return s.layoutLabel(gtx, "Free", textLabelSmall, font.SemiBold, s.theme.onSuccessContainer, 1)
														})
													})
												})
											}
											return layout.Dimensions{}
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if p.RequiresKey {
												if p.HasKey {
													return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														return s.roundedSurface(gtx, shapeSmall, s.theme.secondaryContainer, func(gtx layout.Context) layout.Dimensions {
															return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																return s.layoutLabel(gtx, "Configured", textLabelSmall, font.Normal, s.theme.onSecondaryContainer, 1)
															})
														})
													})
												}
												return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerHighest, func(gtx layout.Context) layout.Dimensions {
														return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return s.layoutLabel(gtx, "API Key Required", textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
														})
													})
												})
											}
											return layout.Dimensions{}
										}),
									)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									sub := p.BaseURL
									if p.DefaultModel != "" {
										sub += " · Default: " + p.DefaultModel
									}
									return desktopInset{Top: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, sub, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
									})
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							chevron := iconChevronRight
							if isSelected {
								chevron = iconChevronDown
							}
							return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, chevron, 14, s.theme.onSurfaceVariant)
							})
						}),
					)
				})
			})
		})
	})
}

func (s *shell) layoutProviderForm(gtx layout.Context, snapshot controllerSnapshot, enabled bool) layout.Dimensions {
	isPreset := false
	for _, preset := range defaultPresetCatalog {
		if strings.EqualFold(preset.ID, s.providerFormSelectedID) {
			isPreset = true
			break
		}
	}

	title := "Configure " + s.providerFormSelectedID
	if !isPreset && s.providerFormSelectedID == "" {
		title = "Add Custom Provider"
	}

	return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerLow, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 12, Bottom: 12, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, title, textTitleMedium, font.SemiBold, s.theme.onSurface, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 8, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						nameEditable := !isPreset && enabled
						return s.layoutProviderFieldEditor(gtx, "Provider Name", &s.providerNameEditor, nameEditable, false)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutProviderFieldEditor(gtx, "Base URL / Endpoint", &s.providerEndpointEditor, enabled, false)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutProtocolSelector(gtx, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutProviderAPIKeyField(gtx, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 4, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutTestConnectionRow(gtx, snapshot, enabled)
					})
				}),
			}

			// Discovered or cached models selector
			models := s.providerDiscoveredModels
			if len(models) == 0 && snapshot.ProviderModels != nil {
				models = snapshot.ProviderModels[strings.ToLower(s.providerFormSelectedID)]
			}
			if len(models) > 0 {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutModelSelectionDropdown(gtx, models, enabled)
					})
				}))
			}

			// Action buttons
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutProviderFormButtons(gtx, isPreset, enabled)
				})
			}))

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

func (s *shell) layoutProtocolSelector(gtx layout.Context, enabled bool) layout.Dimensions {
	isOpenAI := strings.ToLower(s.providerSelectedType) != "anthropic"

	if enabled && s.providerTypeOpenAIBtn.Clicked(gtx) {
		s.providerSelectedType = "openai"
	}
	if enabled && s.providerTypeAnthropicBtn.Clicked(gtx) {
		s.providerSelectedType = "anthropic"
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Protocol:", textLabelMedium, font.Medium, s.theme.onSurfaceVariant, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bg := s.theme.surfaceContainerLow
			fg := s.theme.onSurfaceVariant
			if isOpenAI {
				bg = s.theme.primaryContainer
				fg = s.theme.onPrimaryContainer
			}
			return s.providerTypeOpenAIBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 4, Bottom: 4, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "OpenAI-compatible", textLabelSmall, font.SemiBold, fg, 1)
					})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bg := s.theme.surfaceContainerLow
			fg := s.theme.onSurfaceVariant
			if !isOpenAI {
				bg = s.theme.primaryContainer
				fg = s.theme.onPrimaryContainer
			}
			return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.providerTypeAnthropicBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 4, Bottom: 4, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Anthropic Messages", textLabelSmall, font.SemiBold, fg, 1)
						})
					})
				})
			})
		}),
	)
}

func (s *shell) layoutProviderFieldEditor(gtx layout.Context, label string, ed *widget.Editor, enabled, isPassword bool) layout.Dimensions {
	ed.ReadOnly = !enabled
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, label, textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surface, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					mEd := material.Editor(s.theme.material, ed, "")
					mEd.TextSize = textBodySmall
					mEd.Color = s.theme.onSurface
					return mEd.Layout(gtx)
				})
			})
		}),
	)
}

func (s *shell) layoutProviderAPIKeyField(gtx layout.Context, enabled bool) layout.Dimensions {
	if enabled && s.providerShowKeyBtn.Clicked(gtx) {
		s.providerShowKey = !s.providerShowKey
	}

	maskChar := rune(0)
	if !s.providerShowKey {
		maskChar = '•'
	}
	s.providerAPIKeyEditor.Mask = maskChar
	s.providerAPIKeyEditor.ReadOnly = !enabled

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "API Key", textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					toggleText := "Show key"
					if s.providerShowKey {
						toggleText = "Hide key"
					}
					return s.providerShowKeyBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, toggleText, textLabelSmall, font.Normal, s.theme.primary, 1)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surface, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					mEd := material.Editor(s.theme.material, &s.providerAPIKeyEditor, "Leave blank to keep existing key")
					mEd.TextSize = textBodySmall
					mEd.Color = s.theme.onSurface
					return mEd.Layout(gtx)
				})
			})
		}),
	)
}

func (s *shell) layoutTestConnectionRow(gtx layout.Context, snapshot controllerSnapshot, enabled bool) layout.Dimensions {
	id := s.providerFormSelectedID
	if id == "" {
		id = strings.TrimSpace(s.providerNameEditor.Text())
	}
	isLoading := snapshot.ProviderModelsLoading != nil && snapshot.ProviderModelsLoading[id]

	if enabled && !isLoading && s.providerTestBtn.Clicked(gtx) {
		s.providerTestStatus = "Connecting & discovering models…"
		s.providerTestError = ""
		endpoint := strings.TrimSpace(s.providerEndpointEditor.Text())
		key := strings.TrimSpace(s.providerAPIKeyEditor.Text())
		proto := s.providerSelectedType
		if s.onFetchProviderModels != nil {
			s.onFetchProviderModels(id, endpoint, key, proto, func(models []string, err error) {
				if err != nil {
					s.providerTestError = err.Error()
					s.providerTestStatus = ""
				} else {
					s.providerDiscoveredModels = models
					s.providerTestStatus = fmt.Sprintf("✓ Connected · %d models found", len(models))
					if len(models) > 0 && s.providerSelectedModel == "" {
						s.providerSelectedModel = models[0]
					}
				}
			})
		}
	}

	btnText := "Test & Fetch Models"
	if isLoading {
		btnText = "Testing…"
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutButton(gtx, &s.providerTestBtn, btnText, enabled && !isLoading, nil)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			statusText := s.providerTestStatus
			fg := s.theme.primary
			if s.providerTestError != "" {
				statusText = "✗ " + s.providerTestError
				fg = s.theme.onErrorContainer
			}
			if statusText == "" {
				return layout.Dimensions{}
			}
			return desktopInset{Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, statusText, textLabelSmall, font.Medium, fg, 2)
			})
		}),
	)
}

func (s *shell) layoutModelSelectionDropdown(gtx layout.Context, models []string, enabled bool) layout.Dimensions {
	if enabled && s.providerModelsDropdownBtn.Clicked(gtx) {
		s.providerModelsDropdownOpen = !s.providerModelsDropdownOpen
	}

	current := s.providerSelectedModel
	if current == "" && len(models) > 0 {
		current = models[0]
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Default Model", textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.providerModelsDropdownBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surface, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, current, textBodySmall, font.Medium, s.theme.onSurface, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								icon := iconChevronDown
								if s.providerModelsDropdownOpen {
									icon = iconChevronUp
								}
								return s.layoutActionIcon(gtx, icon, 12, s.theme.onSurfaceVariant)
							}),
						)
					})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !s.providerModelsDropdownOpen {
				return layout.Dimensions{}
			}
			rows := make([]layout.FlexChild, 0, min(8, len(models)))
			for _, m := range models {
				modelID := m
				btn := s.providerModelSelectButton(modelID)
				if enabled && btn.Clicked(gtx) {
					s.providerSelectedModel = modelID
					s.providerModelsDropdownOpen = false
				}
				isSelected := modelID == current
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					bg := color.NRGBA{}
					fg := s.theme.onSurface
					if isSelected {
						bg = s.theme.primaryContainer
						fg = s.theme.onPrimaryContainer
					}
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, modelID, textBodySmall, font.Normal, fg, 1)
							})
						})
					})
				}))
			}
			return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return desktopUniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
					})
				})
			})
		}),
	)
}

func (s *shell) layoutProviderFormButtons(gtx layout.Context, isPreset, enabled bool) layout.Dimensions {
	providerName := strings.TrimSpace(s.providerNameEditor.Text())
	if providerName == "" {
		providerName = s.providerFormSelectedID
	}

	canSave := providerName != "" && enabled

	if canSave && s.providerSaveActivateBtn.Clicked(gtx) {
		s.submitProviderSave(providerName, true)
	}
	if canSave && s.providerSaveBtn.Clicked(gtx) {
		s.submitProviderSave(providerName, false)
	}
	if enabled && s.providerCancelBtn.Clicked(gtx) {
		s.providerFormVisible = false
		s.providerFormSelectedID = ""
	}
	if enabled && s.providerDeleteBtn.Clicked(gtx) {
		if s.onDeleteProvider != nil {
			s.onDeleteProvider(providerName, func(err error) {
				if err == nil {
					s.providerFormVisible = false
					s.providerFormSelectedID = ""
				}
			})
		}
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPrimaryButton(gtx, &s.providerSaveActivateBtn, "Save & Activate", canSave, nil)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &s.providerSaveBtn, "Save", canSave, nil)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &s.providerCancelBtn, "Cancel", enabled, nil)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Spacer{}.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !isPreset && s.providerFormSelectedID != "" {
				return s.layoutDangerButton(gtx, &s.providerDeleteBtn, "Delete Provider", enabled, nil)
			}
			return layout.Dimensions{}
		}),
	)
}

func (s *shell) submitProviderSave(name string, activate bool) {
	params := acpProvidersSaveParams{
		ProviderName: name,
		ProviderType: s.providerSelectedType,
		PreviousName: s.providerFormSelectedID,
		BaseURL:      strings.TrimSpace(s.providerEndpointEditor.Text()),
		APIKey:       strings.TrimSpace(s.providerAPIKeyEditor.Text()),
		DefaultModel: strings.TrimSpace(s.providerSelectedModel),
		Activate:     activate,
	}
	if s.onSaveProvider != nil {
		s.onSaveProvider(params, func(err error) {
			if err == nil {
				s.providerFormVisible = false
				s.providerFormSelectedID = ""
			}
		})
	}
}
