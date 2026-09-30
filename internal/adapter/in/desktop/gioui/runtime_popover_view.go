//go:build desktop || desktop_gio

package gioui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/phongsathornpt/protonman/internal/app"
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

var popoverPermissionModeOptions = []struct {
	Mode  string
	Label string
	Desc  string
}{
	{Mode: "ask", Label: "Ask (Default)", Desc: "Interactive prompts for tool calls and sensitive operations"},
	{Mode: "plan", Label: "Plan (Read-only)", Desc: "Read-only inspection; mutating tools and commands blocked"},
	{Mode: "always-approve", Label: "Always Approve", Desc: "Autonomous execution without confirmation prompts"},
}

func (s *shell) openModelPopover() {
	if !s.modelPopoverVisible && !s.reasoningPopoverVisible && !s.permissionModePopoverVisible && !s.conversationList.Position.BeforeEnd {
		s.tailFollowBeforeOverlay = true
	}
	s.modelPopoverVisible = true
	s.reasoningPopoverVisible = false
	s.permissionModePopoverVisible = false
	s.modelSearchFocusPending = true
	s.mentionActive = false
	s.popoverActiveProviderTab = ""
	if s.onRefreshRuntime != nil {
		s.onRefreshRuntime()
	}
	if s.onRefreshProviders != nil {
		s.onRefreshProviders()
	}
}

func (s *shell) openReasoningPopover() {
	wasAtTail := !s.conversationList.Position.BeforeEnd
	s.closePopovers()
	if wasAtTail {
		s.tailFollowBeforeOverlay = true
	}
	s.reasoningPopoverVisible = true
	s.mentionActive = false
}

func (s *shell) openPermissionModePopover() {
	wasAtTail := !s.conversationList.Position.BeforeEnd
	s.closePopovers()
	if wasAtTail {
		s.tailFollowBeforeOverlay = true
	}
	s.permissionModePopoverVisible = true
	s.mentionActive = false
}

func (s *shell) closePopovers() {
	s.modelPopoverVisible = false
	s.reasoningPopoverVisible = false
	s.permissionModePopoverVisible = false
	s.modelSearchFocusPending = false
	if s.tailFollowBeforeOverlay && !s.mentionActive {
		s.conversationList.ScrollToEnd = true
		s.conversationList.Position = layout.Position{}
		s.tailFollowBeforeOverlay = false
	}
}

func (s *shell) popoverPermissionModeButton(mode string) *widget.Clickable {
	btn, ok := s.popoverPermissionModeButtons[mode]
	if !ok {
		btn = new(widget.Clickable)
		if s.popoverPermissionModeButtons == nil {
			s.popoverPermissionModeButtons = make(map[string]*widget.Clickable)
		}
		s.popoverPermissionModeButtons[mode] = btn
	}
	return btn
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

func (s *shell) popoverProviderTabButton(id string) *widget.Clickable {
	btn, ok := s.popoverProviderTabButtons[id]
	if !ok {
		btn = new(widget.Clickable)
		if s.popoverProviderTabButtons == nil {
			s.popoverProviderTabButtons = make(map[string]*widget.Clickable)
		}
		s.popoverProviderTabButtons[id] = btn
	}
	return btn
}

func fallbackModelsForProvider(provider string) []string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "protonman":
		return []string{"claude-3-7-sonnet-20250219", "claude-3-5-sonnet-20241022", "gpt-4o", "o3-mini", "deepseek-reasoner"}
	case "openai":
		return []string{"gpt-4o", "gpt-4o-mini", "o3-mini", "o1"}
	case "anthropic":
		return []string{"claude-3-7-sonnet-20250219", "claude-3-5-sonnet-20241022", "claude-3-5-haiku-20241022"}
	case "opencode":
		return []string{"nemotron-3-super-free", "big-pickle", "mimo-v2.5-free"}
	case "opencode-zen":
		return []string{"claude-3-7-sonnet-20250219", "gpt-4o", "o3-mini"}
	case "opencode-go":
		return []string{"claude-3-7-sonnet-20250219", "gpt-4o"}
	case "ollama":
		return []string{"llama3.3", "qwen2.5-coder", "deepseek-r1"}
	default:
		return []string{"claude-3-7-sonnet-20250219", "gpt-4o"}
	}
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
		s.closePopovers()
		gtx.Execute(key.FocusCmd{Tag: &s.composer})
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
	isProton := isProtonmanAgent(agentID)

	if currentModel == "" && snapshot.AgentDefaultModels != nil {
		currentModel = strings.TrimSpace(snapshot.AgentDefaultModels[agentID])
	}

	activeProvider := currentProvider
	if isProton {
		if strings.TrimSpace(s.popoverActiveProviderTab) != "" {
			activeProvider = strings.TrimSpace(s.popoverActiveProviderTab)
		} else if activeProvider == "" {
			activeProvider = strings.TrimSpace(snapshot.ActiveProvider)
		}
		if activeProvider == "" {
			activeProvider = "protonman"
		}
		s.popoverActiveProviderTab = activeProvider
	}

	searchQuery := strings.ToLower(strings.TrimSpace(s.modelSearchEditor.Text()))
	var rawModels []string
	if isProton {
		if strings.EqualFold(activeProvider, currentProvider) && len(session.AvailableModels) > 0 {
			rawModels = session.AvailableModels
		} else if cached, ok := snapshot.ProviderModels[strings.ToLower(activeProvider)]; ok && len(cached) > 0 {
			rawModels = cached
		} else {
			rawModels = fallbackModelsForProvider(activeProvider)
		}
	} else {
		rawModels = session.AvailableModels
		if len(rawModels) == 0 && snapshot.AgentAvailableModels != nil {
			rawModels = snapshot.AgentAvailableModels[agentID]
		}
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
				children := make([]layout.FlexChild, 0, 6)

				// Header with agent display name, manage button, and close button
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutModelPopoverHeader(gtx, agentName, isProton)
				}))

				// Divider
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 4, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutHorizontalDivider(gtx)
					})
				}))

				// Provider tabs for Protonman
				if isProton {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutPopoverProviderTabs(gtx, snapshot.Providers, activeProvider, enabled)
					}))
				}

				// Search input when the agent advertises models
				if len(rawModels) > 0 {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutModelSearchInput(gtx)
						})
					}))
				}

				supportsFreeGrouping := isClineProvider(agentID, currentProvider, rawModels, snapshot.AgentProfiles) && hasAnyFreeModel(rawModels)
				var modelItems []modelPopoverListItem
				if supportsFreeGrouping {
					freeFiltered, otherFiltered := partitionModels(filteredModels)
					sort.Slice(freeFiltered, func(i, j int) bool {
						return strings.ToLower(freeFiltered[i]) < strings.ToLower(freeFiltered[j])
					})
					sort.Slice(otherFiltered, func(i, j int) bool {
						return strings.ToLower(otherFiltered[i]) < strings.ToLower(otherFiltered[j])
					})

					if len(freeFiltered) > 0 {
						modelItems = append(modelItems, modelPopoverListItem{
							kind:  modelItemHeader,
							title: "Free Models",
							count: len(freeFiltered),
						})
						for _, m := range freeFiltered {
							modelItems = append(modelItems, modelPopoverListItem{
								kind:    modelItemEntry,
								modelID: m,
								isFree:  true,
							})
						}
					}
					if len(otherFiltered) > 0 {
						modelItems = append(modelItems, modelPopoverListItem{
							kind:  modelItemHeader,
							title: "Other Models",
							count: len(otherFiltered),
						})
						for _, m := range otherFiltered {
							modelItems = append(modelItems, modelPopoverListItem{
								kind:    modelItemEntry,
								modelID: m,
								isFree:  false,
							})
						}
					}
				} else {
					for _, m := range filteredModels {
						modelItems = append(modelItems, modelPopoverListItem{
							kind:    modelItemEntry,
							modelID: m,
							isFree:  false,
						})
					}
				}

				effectiveSelectedModel := currentModel
				if isProton && !strings.EqualFold(activeProvider, currentProvider) {
					effectiveSelectedModel = ""
				}

				targetProvider := currentProvider
				if isProton {
					targetProvider = activeProvider
				}

				// Content: empty state, no search matches, or scrollable model list
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if len(rawModels) == 0 {
						return s.layoutAgentModelsEmptyState(gtx, agentName, enabled)
					}
					if len(filteredModels) == 0 {
						return s.layoutModelSearchNoMatch(gtx, s.modelSearchEditor.Text())
					}
					return s.layoutAgentModelsList(gtx, modelItems, effectiveSelectedModel, targetProvider, enabled)
				}))

				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

func (s *shell) layoutModelPopoverHeader(gtx layout.Context, agentName string, isProton bool) layout.Dimensions {
	if isProton && s.popoverManageProvidersBtn.Clicked(gtx) {
		s.settingsActiveTab = 1
		s.closePopovers()
		s.openSettingsModal()
	}

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
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !isProton {
				return layout.Dimensions{}
			}
			btn := &s.popoverManageProvidersBtn
			semantic.Button.Add(gtx.Ops)
			semantic.DescriptionOp("Manage Model Providers in Settings").Add(gtx.Ops)
			return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				fg := s.theme.primary
				bg := s.theme.surfaceContainerLow
				if btn.Hovered() {
					bg = s.theme.surfaceContainerHighest
				}
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "+ Manage", textLabelSmall, font.Medium, fg, 1)
						})
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

func (s *shell) layoutPopoverProviderTabs(gtx layout.Context, providers []desktopstate.ProviderState, activeProvider string, enabled bool) layout.Dimensions {
	type providerTabItem struct {
		ID       string
		Name     string
		IsFree   bool
		IsActive bool
	}

	tabs := make([]providerTabItem, 0, len(providers)+7)
	if len(providers) > 0 {
		for _, p := range providers {
			tabs = append(tabs, providerTabItem{
				ID:       p.ID,
				Name:     p.Name,
				IsFree:   p.IsFree,
				IsActive: p.IsActive,
			})
		}
	} else {
		tabs = []providerTabItem{
			{ID: "protonman", Name: "Protonman", IsActive: true},
			{ID: "opencode", Name: "OpenCode Free", IsFree: true},
			{ID: "opencode-zen", Name: "OpenCode Zen"},
			{ID: "opencode-go", Name: "OpenCode Go"},
			{ID: "ollama", Name: "Ollama"},
			{ID: "openai", Name: "OpenAI"},
			{ID: "anthropic", Name: "Anthropic"},
		}
	}

	s.popoverProviderTabList.Axis = layout.Horizontal
	return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.popoverProviderTabList.Layout(gtx, len(tabs), func(gtx layout.Context, index int) layout.Dimensions {
			tab := tabs[index]
			selected := strings.EqualFold(tab.ID, activeProvider)
			btn := s.popoverProviderTabButton(tab.ID)

			if enabled && btn.Clicked(gtx) {
				s.popoverActiveProviderTab = tab.ID
				s.modelSearchEditor.SetText("")
				if s.onFetchProviderModels != nil {
					s.onFetchProviderModels(tab.ID, "", "", "", nil)
				}
			}

			semantic.Button.Add(gtx.Ops)
			semantic.EnabledOp(enabled).Add(gtx.Ops)
			semantic.SelectedOp(selected).Add(gtx.Ops)
			semantic.DescriptionOp("Switch provider to " + tab.Name).Add(gtx.Ops)

			bg := s.theme.surfaceContainerLow
			fg := s.theme.onSurfaceVariant
			if selected {
				bg = s.theme.primaryContainer
				fg = s.theme.onPrimaryContainer
			} else if enabled && btn.Hovered() {
				bg = s.theme.surfaceContainerHighest
				fg = s.theme.onSurface
			}
			if !enabled {
				gtx = gtx.Disabled()
			}

			return desktopInset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					dims := s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									fWeight := font.Normal
									if selected {
										fWeight = font.SemiBold
									}
									return s.layoutLabel(gtx, tab.Name, textLabelSmall, fWeight, fg, 1)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if !tab.IsFree {
										return layout.Dimensions{}
									}
									return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										badgeBg := s.theme.successContainer
										badgeFg := s.theme.onSuccessContainer
										return s.roundedSurface(gtx, shapeSmall, badgeBg, func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 1, Bottom: 1, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, "Free", textLabelSmall, font.Bold, badgeFg, 1)
											})
										})
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
		})
	})
}

func (s *shell) layoutModelSearchInput(gtx layout.Context) layout.Dimensions {
	if s.modelSearchFocusPending {
		s.modelSearchFocusPending = false
		gtx.Execute(key.FocusCmd{Tag: &s.modelSearchEditor})
	}

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

type modelPopoverItemKind int

const (
	modelItemHeader modelPopoverItemKind = iota
	modelItemEntry
)

type modelPopoverListItem struct {
	kind    modelPopoverItemKind
	title   string
	count   int
	modelID string
	isFree  bool
}

func isProtonmanAgent(agentID string) bool {
	lower := strings.ToLower(strings.TrimSpace(agentID))
	return lower == "" || lower == controllerAgentID || lower == "proton"
}

func isClineProvider(agentID, provider string, models []string, profiles []app.ACPAgentProfile) bool {
	if isProtonmanAgent(agentID) {
		return false
	}
	lowerAgent := strings.ToLower(strings.TrimSpace(agentID))
	if strings.Contains(lowerAgent, "cline") {
		return true
	}
	lowerProv := strings.ToLower(strings.TrimSpace(provider))
	if strings.Contains(lowerProv, "cline") {
		return true
	}
	for _, prof := range profiles {
		if strings.EqualFold(prof.ID, agentID) {
			if strings.Contains(strings.ToLower(prof.DisplayName), "cline") ||
				strings.Contains(strings.ToLower(prof.Command), "cline") {
				return true
			}
		}
	}
	for _, m := range models {
		lower := strings.ToLower(strings.TrimSpace(m))
		if strings.HasPrefix(lower, "cline/") || strings.HasPrefix(lower, "cline-free/") {
			return true
		}
	}
	return false
}

func isFreeModel(modelID string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	if lower == "" {
		return false
	}
	// Protonman models are never free.
	if strings.HasPrefix(lower, "proton/") || strings.HasPrefix(lower, "protonman/") {
		return false
	}
	if strings.HasSuffix(lower, ":free") ||
		strings.Contains(lower, ":free") ||
		strings.HasSuffix(lower, "/free") ||
		strings.HasSuffix(lower, "-free") ||
		strings.HasSuffix(lower, "(free)") ||
		strings.Contains(lower, "(free)") ||
		strings.Contains(lower, "cline-free") ||
		lower == "free" ||
		lower == "big-pickle" {
		return true
	}
	// Curated free-tier models provided by Cline without an explicit :free suffix
	if strings.HasPrefix(lower, "cline/") || strings.HasPrefix(lower, "stealth/") || strings.HasPrefix(lower, "cline-free/") {
		base := lower
		if idx := strings.LastIndex(base, "/"); idx != -1 {
			base = base[idx+1:]
		}
		switch base {
		case "muse-spark-1.3-contributor",
			"deepseek-v4.1-flash",
			"mimo-v2.6-flash",
			"pixel-canary",
			"space-bunny-alpha",
			"kat-coder-pro",
			"big-pickle":
			return true
		}
	}
	return false
}

func hasAnyFreeModel(models []string) bool {
	for _, m := range models {
		if isFreeModel(m) {
			return true
		}
	}
	return false
}

func partitionModels(models []string) (freeModels []string, otherModels []string) {
	for _, m := range models {
		if isFreeModel(m) {
			freeModels = append(freeModels, m)
		} else {
			otherModels = append(otherModels, m)
		}
	}
	return freeModels, otherModels
}

func (s *shell) layoutAgentModelsList(gtx layout.Context, items []modelPopoverListItem, currentModel, provider string, enabled bool) layout.Dimensions {
	s.modelList.Axis = layout.Vertical
	return s.modelList.Layout(gtx, len(items), func(gtx layout.Context, index int) layout.Dimensions {
		item := items[index]
		if item.kind == modelItemHeader {
			return s.layoutModelSectionHeader(gtx, item.title, item.count)
		}
		selected := item.modelID == currentModel
		return desktopInset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutModelListItem(gtx, item.modelID, provider, selected, item.isFree, enabled)
		})
	})
}

func (s *shell) layoutModelSectionHeader(gtx layout.Context, title string, count int) layout.Dimensions {
	return desktopInset{Top: 6, Bottom: 4, Left: 2, Right: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, title, textLabelSmall, font.SemiBold, s.theme.onSurfaceVariant, 1)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				countText := fmt.Sprintf("%d", count)
				return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerHighest, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Top: 1, Bottom: 1, Left: 5, Right: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, countText, textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
						})
					})
				})
			}),
		)
	})
}

func (s *shell) layoutModelListItem(gtx layout.Context, modelID, provider string, selected, isFree, enabled bool) layout.Dimensions {
	btn := s.agentModelButton(modelID)
	if enabled && btn.Clicked(gtx) {
		if s.onSetRuntimeModel != nil {
			s.onSetRuntimeModel(provider, modelID)
		}
		s.closePopovers()
		gtx.Execute(key.FocusCmd{Tag: &s.composer})
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
						if isFree {
							return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
		s.closePopovers()
		gtx.Execute(key.FocusCmd{Tag: &s.composer})
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
								s.closePopovers()
								gtx.Execute(key.FocusCmd{Tag: &s.composer})
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

func (s *shell) layoutPermissionModePopover(gtx layout.Context, session desktopstate.SessionState, enabled bool) layout.Dimensions {
	if s.permissionModePopoverCloseBtn.Clicked(gtx) {
		s.closePopovers()
		gtx.Execute(key.FocusCmd{Tag: &s.composer})
		return layout.Dimensions{}
	}

	currentMode := strings.TrimSpace(session.Runtime.PermissionMode)
	if currentMode == "" {
		currentMode = "ask"
	}

	return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Permission Mode", textLabelLarge, font.SemiBold, s.theme.onSurface, 1)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "Alt+P · Esc", textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := &s.permissionModePopoverCloseBtn
								semantic.Button.Add(gtx.Ops)
								semantic.DescriptionOp("Close permission mode selector").Add(gtx.Ops)
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
						rows := make([]layout.FlexChild, 0, len(popoverPermissionModeOptions))
						for _, opt := range popoverPermissionModeOptions {
							o := opt
							selected := o.Mode == currentMode
							btn := s.popoverPermissionModeButton(o.Mode)
							if enabled && btn.Clicked(gtx) {
								s.onSetRuntimePermissionMode(o.Mode)
								s.closePopovers()
								gtx.Execute(key.FocusCmd{Tag: &s.composer})
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
