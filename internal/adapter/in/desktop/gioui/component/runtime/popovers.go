//go:build desktop || desktop_gio

package runtime

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

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/modelcatalogpolicy"
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

type ModelPopoverInput struct {
	Session               desktopstate.SessionState
	ActiveAgentID         string
	AgentProfiles         []app.ACPAgentProfile
	AgentDefaultModels    map[string]string
	ActiveProvider        string
	Providers             []desktopstate.ProviderState
	ProviderModels        map[string][]string
	AgentAvailableModels  map[string][]string
	Enabled               bool
	Chrome                Chrome
	OnSetModel            func(provider, model string)
	OnFetchProviderModels func(id, name, baseURL, apiKey string, cb func([]string, error))
	OnRefreshRuntime      func()
	OnManageProviders     func()
	OnClose               func()
}

type ReasoningPopoverInput struct {
	Session        desktopstate.SessionState
	Enabled        bool
	Chrome         Chrome
	OnSetReasoning func(level string)
	OnClose        func()
}

type PermissionModePopoverInput struct {
	Session             desktopstate.SessionState
	Enabled             bool
	Chrome              Chrome
	OnSetPermissionMode func(mode string)
	OnClose             func()
}

func IsProtonmanAgent(agentID string) bool {
	lower := strings.ToLower(strings.TrimSpace(agentID))
	return lower == "" || lower == "protonman" || lower == "proton"
}

func IsClineProvider(agentID, provider string, models []string, profiles []app.ACPAgentProfile) bool {
	if IsProtonmanAgent(agentID) {
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

func agentDisplayName(profiles []app.ACPAgentProfile, agentID string) string {
	agentID = strings.TrimSpace(agentID)
	for _, profile := range profiles {
		if profile.ID == agentID {
			return profile.DisplayName
		}
	}
	if agentID == "" {
		return "Not configured"
	}
	return agentID
}

func (c *Component) LayoutModelPopover(gtx layout.Context, input ModelPopoverInput) layout.Dimensions {
	if c.widgets.ModelPopoverCloseButton.Clicked(gtx) {
		c.ClosePopovers()
		if input.OnClose != nil {
			input.OnClose()
		}
		return layout.Dimensions{}
	}

	maxHeight := gtx.Dp(320)
	gtx.Constraints.Max.Y = maxHeight

	currentModel := strings.TrimSpace(input.Session.Runtime.Model)
	currentProvider := strings.TrimSpace(input.Session.Runtime.Provider)
	agentID := strings.TrimSpace(input.Session.AgentID)
	if agentID == "" {
		agentID = strings.TrimSpace(input.ActiveAgentID)
	}
	if agentID == "" {
		agentID = "protonman"
	}
	agentName := agentDisplayName(input.AgentProfiles, agentID)
	isProton := IsProtonmanAgent(agentID)

	if currentModel == "" && input.AgentDefaultModels != nil {
		currentModel = strings.TrimSpace(input.AgentDefaultModels[agentID])
	}

	activeProvider := currentProvider
	if isProton {
		if strings.TrimSpace(c.widgets.PopoverActiveProviderTab) != "" {
			activeProvider = strings.TrimSpace(c.widgets.PopoverActiveProviderTab)
		} else if activeProvider == "" {
			activeProvider = strings.TrimSpace(input.ActiveProvider)
		}
		if activeProvider == "" {
			activeProvider = "protonman"
		}
		c.widgets.PopoverActiveProviderTab = activeProvider
	}

	searchQuery := strings.ToLower(strings.TrimSpace(c.widgets.ModelSearchEditor.Text()))
	var rawModels []string
	if isProton {
		if strings.EqualFold(activeProvider, currentProvider) && len(input.Session.AvailableModels) > 0 {
			rawModels = input.Session.AvailableModels
		} else if cached, ok := input.ProviderModels[strings.ToLower(activeProvider)]; ok && len(cached) > 0 {
			rawModels = cached
		} else {
			rawModels = modelcatalogpolicy.FallbackModelsForProvider(activeProvider)
		}
	} else {
		rawModels = input.Session.AvailableModels
		if len(rawModels) == 0 && input.AgentAvailableModels != nil {
			rawModels = input.AgentAvailableModels[agentID]
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

	chrome := input.Chrome
	return uikit.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, uikit.ShapeMedium, chrome.Colors.SurfaceContainerHigh, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, 6)

				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.layoutModelPopoverHeader(gtx, agentName, isProton, chrome, input.OnManageProviders)
				}))

				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 4, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.Divider(gtx)
					})
				}))

				if isProton {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.layoutPopoverProviderTabs(gtx, providerTabsInput{
							Providers:      input.Providers,
							ActiveProvider: activeProvider,
							Enabled:        input.Enabled,
							Chrome:         chrome,
							OnFetch:        input.OnFetchProviderModels,
						})
					}))
				}

				if len(rawModels) > 0 {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return c.layoutModelSearchInput(gtx, chrome)
						})
					}))
				}

				supportsFreeGrouping := IsClineProvider(agentID, currentProvider, rawModels, input.AgentProfiles) && modelcatalogpolicy.HasAnyFreeModel(rawModels)
				var modelItems []modelPopoverListItem
				if supportsFreeGrouping {
					freeFiltered, otherFiltered := modelcatalogpolicy.PartitionModels(filteredModels)
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

				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if len(rawModels) == 0 {
						return c.layoutAgentModelsEmptyState(gtx, agentName, input.Enabled, chrome, input.OnRefreshRuntime)
					}
					if len(filteredModels) == 0 {
						return c.layoutModelSearchNoMatch(gtx, c.widgets.ModelSearchEditor.Text(), chrome)
					}
					return c.layoutAgentModelsList(gtx, modelItems, modelListInput{
						Provider:   targetProvider,
						CurrentID:  effectiveSelectedModel,
						Enabled:    input.Enabled,
						Chrome:     chrome,
						OnSetModel: input.OnSetModel,
						OnClose:    input.OnClose,
					})
				}))

				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	})
}

func (c *Component) layoutModelPopoverHeader(gtx layout.Context, agentName string, isProton bool, chrome Chrome, onManage func()) layout.Dimensions {
	if isProton && c.widgets.PopoverManageProvidersButton.Clicked(gtx) && onManage != nil {
		onManage()
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "Select Model", uikit.TextLabelLarge, font.SemiBold, chrome.Colors.OnSurface, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return chrome.RoundedSurface(gtx, uikit.ShapeSmall, chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Top: 2, Bottom: 2, Left: 7, Right: 7}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, agentName, uikit.TextLabelSmall, font.Medium, chrome.Colors.Primary, 1)
					})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !isProton {
				return layout.Dimensions{}
			}
			btn := &c.widgets.PopoverManageProvidersButton
			semantic.Button.Add(gtx.Ops)
			semantic.DescriptionOp("Manage Model Providers in Settings").Add(gtx.Ops)
			return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				fg := chrome.Colors.Primary
				bg := chrome.Colors.SurfaceContainerLow
				if btn.Hovered() {
					bg = chrome.Colors.SurfaceContainerHighest
				}
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, "+ Manage", uikit.TextLabelSmall, font.Medium, fg, 1)
						})
					})
				})
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, "Alt+M · Esc", uikit.TextLabelSmall, font.Normal, chrome.Colors.OnSurfaceVariant, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btn := &c.widgets.ModelPopoverCloseButton
			semantic.Button.Add(gtx.Ops)
			semantic.DescriptionOp("Close model selector").Add(gtx.Ops)
			return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				fg := chrome.Colors.OnSurfaceVariant
				if btn.Hovered() {
					fg = chrome.Colors.OnSurface
				}
				return uikit.Inset{Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return chrome.ActionIcon(gtx, uikit.IconClose, 12, fg)
				})
			})
		}),
	)
}

// providerTabsInput groups the provider-tab row state. The tabs are a
// presentation concern, so the provider list itself stays typed by the caller.
type providerTabsInput struct {
	Providers      []desktopstate.ProviderState
	ActiveProvider string
	Enabled        bool
	Chrome         Chrome
	OnFetch        func(id, name, baseURL, apiKey string, cb func([]string, error))
}

func (c *Component) layoutPopoverProviderTabs(gtx layout.Context, in providerTabsInput) layout.Dimensions {
	type providerTabItem struct {
		ID       string
		Name     string
		IsFree   bool
		IsActive bool
	}

	tabs := make([]providerTabItem, 0, len(in.Providers)+7)
	if len(in.Providers) > 0 {
		for _, p := range in.Providers {
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

	chrome := in.Chrome
	c.widgets.PopoverProviderTabList.Axis = layout.Horizontal
	return uikit.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return c.widgets.PopoverProviderTabList.Layout(gtx, len(tabs), func(gtx layout.Context, index int) layout.Dimensions {
			tab := tabs[index]
			selected := strings.EqualFold(tab.ID, in.ActiveProvider)
			btn := c.PopoverProviderTabButton(tab.ID)

			if in.Enabled && btn.Clicked(gtx) {
				c.widgets.PopoverActiveProviderTab = tab.ID
				c.widgets.ModelSearchEditor.SetText("")
				if in.OnFetch != nil {
					in.OnFetch(tab.ID, "", "", "", nil)
				}
			}

			semantic.Button.Add(gtx.Ops)
			semantic.EnabledOp(in.Enabled).Add(gtx.Ops)
			semantic.SelectedOp(selected).Add(gtx.Ops)
			semantic.DescriptionOp("Switch provider to " + tab.Name).Add(gtx.Ops)

			bg := chrome.Colors.SurfaceContainerLow
			fg := chrome.Colors.OnSurfaceVariant
			if selected {
				bg = chrome.Colors.PrimaryContainer
				fg = chrome.Colors.OnPrimaryContainer
			} else if in.Enabled && btn.Hovered() {
				bg = chrome.Colors.SurfaceContainerHighest
				fg = chrome.Colors.OnSurface
			}
			if !in.Enabled {
				gtx = gtx.Disabled()
			}

			return uikit.Inset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					dims := chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									fWeight := font.Normal
									if selected {
										fWeight = font.SemiBold
									}
									return chrome.Label(gtx, tab.Name, uikit.TextLabelSmall, fWeight, fg, 1)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if !tab.IsFree {
										return layout.Dimensions{}
									}
									return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										badgeBg := chrome.Colors.SuccessContainer
										badgeFg := chrome.Colors.OnSuccessContainer
										return chrome.RoundedSurface(gtx, uikit.ShapeSmall, badgeBg, func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Top: 1, Bottom: 1, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return chrome.Label(gtx, "Free", uikit.TextLabelSmall, font.Bold, badgeFg, 1)
											})
										})
									})
								}),
							)
						})
					})
					if selected {
						widget.Border{Color: chrome.Colors.Primary, CornerRadius: uikit.ShapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: dims.Size}
						})
					}
					return dims
				})
			})
		})
	})
}

func (c *Component) layoutModelSearchInput(gtx layout.Context, chrome Chrome) layout.Dimensions {
	if c.widgets.ModelSearchFocusPending {
		c.widgets.ModelSearchFocusPending = false
		gtx.Execute(key.FocusCmd{Tag: &c.widgets.ModelSearchEditor})
	}

	for {
		evt, ok := gtx.Event(key.Filter{Focus: &c.widgets.ModelSearchEditor, Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			c.widgets.ModelSearchEditor.SetText("")
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}
	if c.widgets.ModelSearchClearButton.Clicked(gtx) {
		c.widgets.ModelSearchEditor.SetText("")
	}

	gtx.Constraints.Min.Y = gtx.Dp(28)
	isFocused := gtx.Focused(&c.widgets.ModelSearchEditor)
	borderColor := color.NRGBA{}
	borderWidth := 0
	iconColor := chrome.Colors.OnSurfaceVariant
	if isFocused {
		borderColor = chrome.Colors.Primary
		borderWidth = 1
		iconColor = chrome.Colors.Primary
	}

	return chrome.BorderSurface(gtx, uikit.ShapeSmall, chrome.Colors.SurfaceContainerLow, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.ActionIcon(gtx, uikit.IconSearch, 14, iconColor)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(chrome.Material, &c.widgets.ModelSearchEditor, "Filter models…")
					ed.TextSize = uikit.TextBodySmall
					ed.Color = chrome.Colors.OnSurface
					ed.HintColor = chrome.Colors.OnSurfaceVariant
					return ed.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if c.widgets.ModelSearchEditor.Text() == "" {
						return layout.Dimensions{}
					}
					return uikit.Inset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.MiniIconButton(gtx, &c.widgets.ModelSearchClearButton, "×", chrome.Colors.OnSurfaceVariant)
					})
				}),
			)
		})
	})
}

func (c *Component) layoutAgentModelsEmptyState(gtx layout.Context, agentName string, enabled bool, chrome Chrome, onRefresh func()) layout.Dimensions {
	if c.widgets.ModelRefreshButton.Clicked(gtx) && enabled && onRefresh != nil {
		onRefresh()
	}

	return chrome.RoundedSurface(gtx, uikit.ShapeSmall, chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 16, Bottom: 16, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, "ⓘ", uikit.TextTitleMedium, font.Normal, chrome.Colors.OnSurfaceVariant, 1)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					msg := fmt.Sprintf("%s manages models internally or does not advertise them over ACP.", agentName)
					return uikit.Inset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, msg, uikit.TextBodySmall, font.Medium, chrome.Colors.OnSurface, 2)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return uikit.Inset{Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, "Configure models directly in the agent or press refresh to query.", uikit.TextLabelSmall, font.Normal, chrome.Colors.OnSurfaceVariant, 2)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := &c.widgets.ModelRefreshButton
					semantic.Button.Add(gtx.Ops)
					semantic.EnabledOp(enabled).Add(gtx.Ops)
					bg := chrome.Colors.SurfaceContainerHighest
					fg := chrome.Colors.OnSurface
					if enabled && btn.Hovered() {
						bg = chrome.Colors.PrimaryContainer
						fg = chrome.Colors.OnPrimaryContainer
					}
					if !enabled {
						gtx = gtx.Disabled()
					}
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.BorderSurface(gtx, uikit.ShapeSmall, bg, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
							return uikit.Inset{Top: 5, Bottom: 5, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, "Refresh models (Alt+M)", uikit.TextLabelSmall, font.SemiBold, fg, 1)
							})
						})
					})
				}),
			)
		})
	})
}

func (c *Component) layoutModelSearchNoMatch(gtx layout.Context, query string, chrome Chrome) layout.Dimensions {
	if c.widgets.ModelSearchClearButton.Clicked(gtx) {
		c.widgets.ModelSearchEditor.SetText("")
	}

	return chrome.RoundedSurface(gtx, uikit.ShapeSmall, chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 16, Bottom: 16, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					msg := fmt.Sprintf("No models match \"%s\"", query)
					return uikit.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, msg, uikit.TextBodySmall, font.Medium, chrome.Colors.OnSurfaceVariant, 1)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := &c.widgets.ModelSearchClearButton
					semantic.Button.Add(gtx.Ops)
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						fg := chrome.Colors.Primary
						if btn.Hovered() {
							fg = chrome.Colors.OnSurface
						}
						return chrome.Label(gtx, "Clear filter", uikit.TextLabelSmall, font.SemiBold, fg, 1)
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

// modelListInput groups the state a model list row needs, so the list and row
// helpers do not thread seven positional arguments through every frame.
type modelListInput struct {
	Provider   string
	CurrentID  string
	Enabled    bool
	Chrome     Chrome
	OnSetModel func(provider, model string)
	OnClose    func()
}

func (c *Component) layoutAgentModelsList(gtx layout.Context, items []modelPopoverListItem, in modelListInput) layout.Dimensions {
	c.widgets.ModelList.Axis = layout.Vertical
	return c.widgets.ModelList.Layout(gtx, len(items), func(gtx layout.Context, index int) layout.Dimensions {
		item := items[index]
		if item.kind == modelItemHeader {
			return c.layoutModelSectionHeader(gtx, item.title, item.count, in.Chrome)
		}
		selected := item.modelID == in.CurrentID
		return uikit.Inset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return c.layoutModelListItem(gtx, item.modelID, selected, item.isFree, in)
		})
	})
}

func (c *Component) layoutModelSectionHeader(gtx layout.Context, title string, count int, chrome Chrome) layout.Dimensions {
	return uikit.Inset{Top: 6, Bottom: 4, Left: 2, Right: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, title, uikit.TextLabelSmall, font.SemiBold, chrome.Colors.OnSurfaceVariant, 1)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				countText := fmt.Sprintf("%d", count)
				return uikit.Inset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return chrome.RoundedSurface(gtx, uikit.ShapeSmall, chrome.Colors.SurfaceContainerHighest, func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 1, Bottom: 1, Left: 5, Right: 5}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, countText, uikit.TextLabelSmall, font.Medium, chrome.Colors.OnSurfaceVariant, 1)
						})
					})
				})
			}),
		)
	})
}

func (c *Component) layoutModelListItem(gtx layout.Context, modelID string, selected, isFree bool, in modelListInput) layout.Dimensions {
	btn := c.AgentModelButton(modelID)
	if in.Enabled && btn.Clicked(gtx) {
		if in.OnSetModel != nil {
			in.OnSetModel(in.Provider, modelID)
		}
		c.ClosePopovers()
		if in.OnClose != nil {
			in.OnClose()
		}
	}

	semantic.Button.Add(gtx.Ops)
	semantic.EnabledOp(in.Enabled).Add(gtx.Ops)
	semantic.SelectedOp(selected).Add(gtx.Ops)
	semantic.DescriptionOp("Select model " + modelID).Add(gtx.Ops)

	chrome := in.Chrome
	bg := chrome.Colors.SurfaceContainerLow
	fg := chrome.Colors.OnSurface
	if selected {
		bg = chrome.Colors.PrimaryContainer
		fg = chrome.Colors.OnPrimaryContainer
	} else if in.Enabled && btn.Hovered() {
		bg = chrome.Colors.SurfaceContainerHighest
	}
	if !in.Enabled {
		gtx = gtx.Disabled()
	}

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		dims := chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if selected {
							return uikit.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, "✓", uikit.TextLabelMedium, font.Bold, fg, 1)
							})
						}
						return layout.Dimensions{}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, modelID, uikit.TextBodySmall, font.Medium, fg, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if isFree {
							return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return chrome.RoundedSurface(gtx, uikit.ShapeSmall, chrome.Colors.SuccessContainer, func(gtx layout.Context) layout.Dimensions {
									return uikit.Inset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return chrome.Label(gtx, "Free", uikit.TextLabelSmall, font.SemiBold, chrome.Colors.OnSuccessContainer, 1)
									})
								})
							})
						}
						return layout.Dimensions{}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if selected {
							return chrome.RoundedSurface(gtx, uikit.ShapeSmall, chrome.Colors.Primary, func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, "Active", uikit.TextLabelSmall, font.SemiBold, chrome.Colors.OnPrimary, 1)
								})
							})
						}
						return layout.Dimensions{}
					}),
				)
			})
		})
		if selected {
			widget.Border{Color: chrome.Colors.Primary, CornerRadius: uikit.ShapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: dims.Size}
			})
		}
		return dims
	})
}

func (c *Component) LayoutReasoningPopover(gtx layout.Context, input ReasoningPopoverInput) layout.Dimensions {
	if c.widgets.ReasoningPopoverCloseButton.Clicked(gtx) {
		c.ClosePopovers()
		if input.OnClose != nil {
			input.OnClose()
		}
		return layout.Dimensions{}
	}

	currentEffort := strings.TrimSpace(input.Session.Runtime.Reasoning)
	if currentEffort == "" {
		currentEffort = "auto"
	}

	chrome := input.Chrome
	return uikit.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, uikit.ShapeMedium, chrome.Colors.SurfaceContainerHigh, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, "Reasoning Effort", uikit.TextLabelLarge, font.SemiBold, chrome.Colors.OnSurface, 1)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, "Alt+R · Esc", uikit.TextLabelSmall, font.Normal, chrome.Colors.OnSurfaceVariant, 1)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := &c.widgets.ReasoningPopoverCloseButton
								semantic.Button.Add(gtx.Ops)
								semantic.DescriptionOp("Close reasoning selector").Add(gtx.Ops)
								return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									fg := chrome.Colors.OnSurfaceVariant
									if btn.Hovered() {
										fg = chrome.Colors.OnSurface
									}
									return uikit.Inset{Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return chrome.ActionIcon(gtx, uikit.IconClose, 12, fg)
									})
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 4, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return chrome.Divider(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						rows := make([]layout.FlexChild, 0, len(popoverReasoningOptions))
						for _, opt := range popoverReasoningOptions {
							o := opt
							selected := o.Level == currentEffort
							btn := c.PopoverReasoningButton(o.Level)
							if input.Enabled && btn.Clicked(gtx) {
								if input.OnSetReasoning != nil {
									input.OnSetReasoning(o.Level)
								}
								c.ClosePopovers()
								if input.OnClose != nil {
									input.OnClose()
								}
							}
							rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									bg := chrome.Colors.SurfaceContainerLow
									fg := chrome.Colors.OnSurface
									descFg := chrome.Colors.OnSurfaceVariant
									if selected {
										bg = chrome.Colors.PrimaryContainer
										fg = chrome.Colors.OnPrimaryContainer
										descFg = chrome.Colors.OnPrimaryContainer
									} else if input.Enabled && btn.Hovered() {
										bg = chrome.Colors.SurfaceContainerHighest
									}
									return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										dims := chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Top: 5, Bottom: 5, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														if selected {
															return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																return chrome.Label(gtx, "✓", uikit.TextLabelSmall, font.Bold, fg, 1)
															})
														}
														return uikit.Inset{Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return layout.Spacer{}.Layout(gtx)
														})
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return chrome.Label(gtx, o.Label, uikit.TextLabelMedium, font.SemiBold, fg, 1)
													}),
													layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
														return uikit.Inset{Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return chrome.Label(gtx, o.Desc, uikit.TextLabelSmall, font.Normal, descFg, 1)
														})
													}),
												)
											})
										})
										if selected {
											widget.Border{Color: chrome.Colors.Primary, CornerRadius: uikit.ShapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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

func (c *Component) LayoutPermissionModePopover(gtx layout.Context, input PermissionModePopoverInput) layout.Dimensions {
	if c.widgets.PermissionModePopoverCloseButton.Clicked(gtx) {
		c.ClosePopovers()
		if input.OnClose != nil {
			input.OnClose()
		}
		return layout.Dimensions{}
	}

	currentMode := strings.TrimSpace(input.Session.Runtime.PermissionMode)
	if currentMode == "" {
		currentMode = "ask"
	}

	chrome := input.Chrome
	return uikit.Inset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, uikit.ShapeMedium, chrome.Colors.SurfaceContainerHigh, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
			return uikit.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, "Permission Mode", uikit.TextLabelLarge, font.SemiBold, chrome.Colors.OnSurface, 1)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, "Alt+P · Esc", uikit.TextLabelSmall, font.Normal, chrome.Colors.OnSurfaceVariant, 1)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := &c.widgets.PermissionModePopoverCloseButton
								semantic.Button.Add(gtx.Ops)
								semantic.DescriptionOp("Close permission mode selector").Add(gtx.Ops)
								return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									fg := chrome.Colors.OnSurfaceVariant
									if btn.Hovered() {
										fg = chrome.Colors.OnSurface
									}
									return uikit.Inset{Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return chrome.ActionIcon(gtx, uikit.IconClose, 12, fg)
									})
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Top: 4, Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return chrome.Divider(gtx)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						rows := make([]layout.FlexChild, 0, len(popoverPermissionModeOptions))
						for _, opt := range popoverPermissionModeOptions {
							o := opt
							selected := o.Mode == currentMode
							btn := c.PopoverPermissionModeButton(o.Mode)
							if input.Enabled && btn.Clicked(gtx) {
								if input.OnSetPermissionMode != nil {
									input.OnSetPermissionMode(o.Mode)
								}
								c.ClosePopovers()
								if input.OnClose != nil {
									input.OnClose()
								}
							}
							rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									bg := chrome.Colors.SurfaceContainerLow
									fg := chrome.Colors.OnSurface
									descFg := chrome.Colors.OnSurfaceVariant
									if selected {
										bg = chrome.Colors.PrimaryContainer
										fg = chrome.Colors.OnPrimaryContainer
										descFg = chrome.Colors.OnPrimaryContainer
									} else if input.Enabled && btn.Hovered() {
										bg = chrome.Colors.SurfaceContainerHighest
									}
									return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										dims := chrome.RoundedSurface(gtx, uikit.ShapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
											return uikit.Inset{Top: 5, Bottom: 5, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														if selected {
															return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																return chrome.Label(gtx, "✓", uikit.TextLabelSmall, font.Bold, fg, 1)
															})
														}
														return uikit.Inset{Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return layout.Spacer{}.Layout(gtx)
														})
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return chrome.Label(gtx, o.Label, uikit.TextLabelMedium, font.SemiBold, fg, 1)
													}),
													layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
														return uikit.Inset{Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return chrome.Label(gtx, o.Desc, uikit.TextLabelSmall, font.Normal, descFg, 1)
														})
													}),
												)
											})
										})
										if selected {
											widget.Border{Color: chrome.Colors.Primary, CornerRadius: uikit.ShapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
