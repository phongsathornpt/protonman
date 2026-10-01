//go:build desktop || desktop_gio

// Package settings owns the desktop settings surface: modal interaction state,
// tab selection, scrolling, and the shared tab/container layout. The shell
// provides each tab's feature content and application callbacks.
package settings

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
)

const (
	GeneralTab = iota
	ProvidersTab
	MCPTab
	AgentsTab
)

var tabLabels = [...]string{"General", "Model Providers", "MCP Integrations", "ACP Agents"}

// Chrome is the shared uikit surface; settings needs no extra primitives.
type Chrome = uikit.Chrome

type ViewInput struct {
	Chrome  Chrome
	Content func(layout.Context, int) layout.Dimensions
	OnClose func()
}

type Component struct {
	open         bool
	activeTab    int
	scrim        widget.Clickable
	card         widget.Clickable
	close        widget.Clickable
	tabButtons   [len(tabLabels)]widget.Clickable
	list         layout.List
	themeButtons map[string]*widget.Clickable
	agentList    layout.List
	agentButtons map[string]*widget.Clickable
	agentOpen    bool
	mcp          MCPWidgets
	provider     ProviderWidgets
	agent        AgentWidgets
}

// ProviderWidgets keeps editor and interaction state for Model Providers in
// the settings feature component rather than in the root shell.
type ProviderWidgets struct {
	FormVisible, ShowKey, ModelsDropdownOpen bool
	FormSelectedID, SelectedType             string
	NameEditor, EndpointEditor, APIKeyEditor widget.Editor
	TypeOpenAIButton, TypeAnthropicButton    widget.Clickable
	ShowKeyButton, TestButton                widget.Clickable
	SaveButton, SaveActivateButton           widget.Clickable
	DeleteButton, CancelButton               widget.Clickable
	AddCustomButton                          widget.Clickable
	CardButtons                              map[string]*widget.Clickable
	TestStatus, TestError                    string
	DiscoveredModels                         []string
	SelectedModel                            string
	ModelSelectButtons                       map[string]*widget.Clickable
	ModelsDropdownButton                     widget.Clickable
}

// AgentWidgets keeps the ACP agent settings editors and interaction state in
// the settings surface component.
type AgentWidgets struct {
	SelectorButton                                                  widget.Clickable
	IDEditor, NameEditor, CommandEditor, ArgsEditor, EnvEditor      widget.Editor
	ProfileButtons                                                  map[string]*widget.Clickable
	ProfileLive                                                     map[string]struct{}
	SyncRevision                                                    uint64
	SyncRevisionSet                                                 bool
	EditorVisible                                                   bool
	EditorOriginalID, EditorKey                                     string
	FormToggleButton, ScanDeviceButton                              widget.Clickable
	SaveButton, RemoveButton, CloseButton, CancelButton             widget.Clickable
	PresetProtonmanButton, PresetOpencodeButton, PresetClineButton  widget.Clickable
	PresetAntigravityButton, PresetClaudeButton, PresetCustomButton widget.Clickable
	ConfirmDelete                                                   bool
	ConfirmDeleteButton, CancelDeleteButton                         widget.Clickable
}

// MCPWidgets stores the editor and interaction state for the MCP integration
// settings view while its renderer is being migrated into this package.
type MCPWidgets struct {
	NameEditor, CommandEditor, ArgsEditor, EnvEditor widget.Editor
	SelectedName, EditorKey                          string
	FormVisible                                      bool
	IntegrationButtons                               map[string]*widget.Clickable
	IntegrationLive                                  map[string]struct{}
	SyncRevision                                     uint64
	SyncRevisionSet                                  bool
	FormToggleButton, SaveButton, RemoveButton       widget.Clickable
	CloseButton, CancelButton                        widget.Clickable
	PresetGitHubButton, PresetMemoryButton           widget.Clickable
	PresetFilesystemButton, PresetFetchButton        widget.Clickable
	PresetCustomButton                               widget.Clickable
	ConfirmDelete                                    bool
	ConfirmDeleteButton, CancelDeleteButton          widget.Clickable
	ReconnectButton                                  widget.Clickable
}

func New() *Component {
	return &Component{
		list: layout.List{Axis: layout.Vertical}, agentList: layout.List{Axis: layout.Horizontal},
		provider: ProviderWidgets{
			NameEditor:     widget.Editor{SingleLine: true, MaxLen: 256},
			EndpointEditor: widget.Editor{SingleLine: true, MaxLen: 2048},
			APIKeyEditor:   widget.Editor{SingleLine: true, MaxLen: 1024},
			CardButtons:    make(map[string]*widget.Clickable), ModelSelectButtons: make(map[string]*widget.Clickable),
		},
		agent: AgentWidgets{
			IDEditor:       widget.Editor{SingleLine: true, MaxLen: 128},
			NameEditor:     widget.Editor{SingleLine: true, MaxLen: 256},
			CommandEditor:  widget.Editor{SingleLine: true, MaxLen: 1024},
			ArgsEditor:     widget.Editor{SingleLine: true, MaxLen: 4096},
			EnvEditor:      widget.Editor{SingleLine: true, MaxLen: 4096},
			ProfileButtons: make(map[string]*widget.Clickable), ProfileLive: make(map[string]struct{}),
		},
		mcp: MCPWidgets{
			NameEditor: widget.Editor{SingleLine: true, MaxLen: 256}, CommandEditor: widget.Editor{SingleLine: true, MaxLen: 1024},
			ArgsEditor: widget.Editor{SingleLine: true, MaxLen: 4096}, EnvEditor: widget.Editor{SingleLine: true, MaxLen: 4096},
			IntegrationButtons: make(map[string]*widget.Clickable),
		},
	}
}

func (c *Component) MCPWidgets() *MCPWidgets           { return &c.mcp }
func (c *Component) ProviderWidgets() *ProviderWidgets { return &c.provider }
func (c *Component) AgentWidgets() *AgentWidgets       { return &c.agent }

type ProviderItem struct {
	ID, Name, Protocol, BaseURL, DefaultModel string
	RequiresKey, HasKey, IsActive, IsFree     bool
}

type ProviderCardInput struct {
	Provider ProviderItem
	Selected bool
	Enabled  bool
	Chrome   Chrome
	OnSelect func(ProviderItem)
}

type ProvidersInput struct {
	Providers []ProviderItem
	Updating  bool
	Chrome    Chrome
	OnAdd     func()
	OnSelect  func(ProviderItem)
	Form      func(layout.Context) layout.Dimensions
}

// LayoutProviders owns the provider tab's summary, provider list, and form
// placement. The shell supplies a form callback while that workflow is moved.
func (c *Component) LayoutProviders(gtx layout.Context, input ProvidersInput) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return input.Chrome.PanelTitle(gtx, "Model Providers") }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return input.Chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
				return input.Chrome.Label(gtx, "Configure API keys, endpoints, and default models for LLM providers.", unit.Sp(12), font.Normal, input.Chrome.Colors.OnSurfaceVariant, 2)
			})
		}),
	}
	for _, provider := range input.Providers {
		provider := provider
		selected := c.provider.FormVisible && c.provider.FormSelectedID == provider.ID
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.LayoutProviderCard(gtx, ProviderCardInput{Provider: provider, Selected: selected, Enabled: !input.Updating, Chrome: input.Chrome, OnSelect: input.OnSelect})
		}))
	}
	if !c.provider.FormVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return input.Chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
				return input.Chrome.Button(gtx, &c.provider.AddCustomButton, "+ Add Custom Provider / Proxy", !input.Updating, input.OnAdd)
			})
		}))
	} else if input.Form != nil {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return input.Chrome.Inset(gtx, layout.Inset{Top: 12, Bottom: 12}, input.Form)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// LayoutProviderCard renders one provider summary card and owns its click state.
func (c *Component) LayoutProviderCard(gtx layout.Context, input ProviderCardInput) layout.Dimensions {
	p, chrome := input.Provider, input.Chrome
	button := c.provider.CardButtons[p.ID]
	if button == nil {
		button = new(widget.Clickable)
		c.provider.CardButtons[p.ID] = button
	}
	if input.Enabled && button.Clicked(gtx) {
		if input.Selected {
			c.provider.FormVisible, c.provider.FormSelectedID = false, ""
		} else if input.OnSelect != nil {
			input.OnSelect(p)
		}
	}
	semantic.Button.Add(gtx.Ops)
	semantic.EnabledOp(input.Enabled).Add(gtx.Ops)
	semantic.DescriptionOp("Configure provider " + p.Name).Add(gtx.Ops)
	bg, border := chrome.Colors.SurfaceContainerLow, chrome.Colors.OutlineVariant
	if input.Selected {
		bg, border = chrome.Colors.SurfaceContainerHigh, chrome.Colors.Primary
	} else if input.Enabled && button.Hovered() {
		bg = chrome.Colors.SurfaceContainerHighest
	}
	return chrome.Inset(gtx, layout.Inset{Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
		return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return chrome.BorderSurface(gtx, unit.Dp(4), bg, border, 1, func(gtx layout.Context) layout.Dimensions {
				return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return chrome.Label(gtx, p.Name, unit.Sp(14), font.SemiBold, chrome.Colors.OnSurface, 1)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if !p.IsActive {
												return layout.Dimensions{}
											}
											return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
												return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.Primary, func(gtx layout.Context) layout.Dimensions {
													return chrome.Inset(gtx, layout.Inset{Top: 1, Bottom: 1, Left: 6, Right: 6}, func(gtx layout.Context) layout.Dimensions {
														return chrome.Label(gtx, "Active Default", unit.Sp(10), font.SemiBold, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, 1)
													})
												})
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if !p.IsFree {
												return layout.Dimensions{}
											}
											return chrome.Inset(gtx, layout.Inset{Left: 6}, func(gtx layout.Context) layout.Dimensions {
												return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.SuccessContainer, func(gtx layout.Context) layout.Dimensions {
													return chrome.Inset(gtx, layout.Inset{Top: 1, Bottom: 1, Left: 6, Right: 6}, func(gtx layout.Context) layout.Dimensions {
														return chrome.Label(gtx, "Free", unit.Sp(10), font.SemiBold, chrome.Colors.OnSuccessContainer, 1)
													})
												})
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if !p.RequiresKey {
												return layout.Dimensions{}
											}
											label, fill, fg := "API Key Required", chrome.Colors.SurfaceContainerHighest, chrome.Colors.OnSurfaceVariant
											if p.HasKey {
												label, fill, fg = "Configured", chrome.Colors.SecondaryContainer, chrome.Colors.OnSecondaryContainer
											}
											return chrome.Inset(gtx, layout.Inset{Left: 6}, func(gtx layout.Context) layout.Dimensions {
												return chrome.RoundedSurface(gtx, unit.Dp(4), fill, func(gtx layout.Context) layout.Dimensions {
													return chrome.Inset(gtx, layout.Inset{Top: 1, Bottom: 1, Left: 6, Right: 6}, func(gtx layout.Context) layout.Dimensions {
														return chrome.Label(gtx, label, unit.Sp(10), font.Normal, fg, 1)
													})
												})
											})
										}),
									)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									sub := p.BaseURL
									if p.DefaultModel != "" {
										sub += " · Default: " + p.DefaultModel
									}
									return chrome.Inset(gtx, layout.Inset{Top: 2}, func(gtx layout.Context) layout.Dimensions {
										return chrome.Label(gtx, sub, unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 1)
									})
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if chrome.ActionIcon != nil {
								icon := uikit.IconChevronRight
								if input.Selected {
									icon = uikit.IconChevronDown
								}
								return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
									return chrome.ActionIcon(gtx, icon, unit.Dp(14), chrome.Colors.OnSurfaceVariant)
								})
							}
							return layout.Dimensions{}
						}),
					)
				})
			})
		})
	})
}

type AgentChoice struct {
	ID, DisplayName, Connection string
}

type AgentSelectorInput struct {
	Profiles []AgentChoice
	ActiveID string
	Chrome   Chrome
	OnSelect func(string)
}

func (c *Component) SyncAgentChoices(ids []string) {
	if c.agentButtons == nil {
		c.agentButtons = make(map[string]*widget.Clickable, len(ids))
	}
	active := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		active[id] = struct{}{}
		if c.agentButtons[id] == nil {
			c.agentButtons[id] = new(widget.Clickable)
		}
	}
	for id := range c.agentButtons {
		if _, ok := active[id]; !ok {
			delete(c.agentButtons, id)
		}
	}
}

func (c *Component) AgentSelectorVisible() bool { return c.agentOpen }
func (c *Component) ToggleAgentSelector()       { c.agentOpen = !c.agentOpen }
func (c *Component) CloseAgentSelector()        { c.agentOpen = false }

func (c *Component) LayoutAgentSelector(gtx layout.Context, input AgentSelectorInput) layout.Dimensions {
	chrome := input.Chrome
	gtx.Constraints.Min.Y = gtx.Dp(56)
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Spacer{}.Layout(gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(320)
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(460))
			return chrome.RoundedSurface(gtx, unit.Dp(8), chrome.Colors.SurfaceContainerHigh, func(gtx layout.Context) layout.Dimensions {
				return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 6, Left: 16, Right: 16}, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, "Choose agent", unit.Sp(14), font.Medium, chrome.Colors.OnSurfaceVariant, 1)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Spacer{}.Layout(gtx) }),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							c.agentList.Axis = layout.Horizontal
							return c.agentList.Layout(gtx, len(input.Profiles), func(gtx layout.Context, index int) layout.Dimensions {
								profile := input.Profiles[index]
								return chrome.Inset(gtx, layout.Inset{Top: 3, Bottom: 3, Left: 3, Right: 3}, func(gtx layout.Context) layout.Dimensions {
									return c.layoutAgentChoice(gtx, profile, profile.ID == input.ActiveID, chrome, input.OnSelect)
								})
							})
						}),
					)
				})
			})
		}),
	)
}

func (c *Component) layoutAgentChoice(gtx layout.Context, profile AgentChoice, selected bool, chrome Chrome, onSelect func(string)) layout.Dimensions {
	button := c.agentButtons[profile.ID]
	if button == nil {
		button = new(widget.Clickable)
		if c.agentButtons == nil {
			c.agentButtons = make(map[string]*widget.Clickable)
		}
		c.agentButtons[profile.ID] = button
	}
	if button.Clicked(gtx) {
		if onSelect != nil {
			onSelect(profile.ID)
		}
		c.agentOpen = false
	}
	background, foreground := chrome.Colors.Surface, chrome.Colors.OnSurface
	if selected || button.Hovered() {
		background, foreground = chrome.Colors.PrimaryContainer, chrome.Colors.OnPrimaryContainer
	}
	gtx.Constraints.Min.X, gtx.Constraints.Min.Y = gtx.Dp(148), gtx.Dp(44)
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(44)
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		semantic.DescriptionOp("Select agent " + profile.DisplayName + ", " + profile.Connection).Add(gtx.Ops)
		return chrome.RoundedSurface(gtx, unit.Dp(4), background, func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, profile.DisplayName+" · "+agentConnectionChip(profile.Connection), unit.Sp(14), font.SemiBold, foreground, 1)
			})
		})
	})
	if gtx.Focused(button) {
		widget.Border{Color: chrome.Colors.Primary, CornerRadius: unit.Dp(4), Width: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: dims.Size} })
	}
	return dims
}

func agentConnectionChip(connection string) string {
	switch connection {
	case "connected":
		return "ready"
	case "reconnecting":
		return "retrying"
	default:
		return "starting"
	}
}

func (c *Component) Show() { c.open = true }

func (c *Component) Close() { c.open = false }

func (c *Component) IsOpen() bool { return c.open }

func (c *Component) ActiveTab() int { return c.activeTab }

func (c *Component) SetActiveTab(tab int) { c.activeTab = max(0, min(tab, len(tabLabels)-1)) }

func (c *Component) Layout(gtx layout.Context, input ViewInput) layout.Dimensions {
	if !c.open {
		return layout.Dimensions{}
	}
	if c.close.Clicked(gtx) || c.scrim.Clicked(gtx) || c.escapePressed(gtx) {
		c.Close()
		if input.OnClose != nil {
			input.OnClose()
		}
		return layout.Dimensions{}
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{A: 160}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return c.scrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			width := min(gtx.Dp(580), gtx.Constraints.Max.X-gtx.Dp(32))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			gtx.Constraints.Max.Y -= gtx.Dp(60)
			return c.card.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return input.Chrome.BorderSurface(gtx, unit.Dp(18), input.Chrome.Colors.SurfaceContainerHigh, input.Chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 16, Bottom: 16, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return c.layoutHeader(gtx, input.Chrome) }),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: 14, Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return c.layoutTabs(gtx, input.Chrome) })
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								c.list.Axis = layout.Vertical
								return c.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
									if input.Content == nil {
										return layout.Dimensions{}
									}
									if gtx.Constraints.Max.X > 0 {
										gtx.Constraints.Min.X = gtx.Constraints.Max.X
									}
									return input.Content(gtx, c.activeTab)
								})
							}),
						)
					})
				})
			})
		}),
	)
}

type GeneralInput struct {
	Theme      string
	SystemDark bool
	Chrome     Chrome
	OnSetTheme func(string)
}

// LayoutGeneral owns the appearance and product-information settings view.
func (c *Component) LayoutGeneral(gtx layout.Context, input GeneralInput) layout.Dimensions {
	chrome := input.Chrome
	currentTheme := input.Theme
	if currentTheme == "" {
		currentTheme = "system"
	}
	systemDesc := "Automatically match your OS appearance (currently Light)"
	if input.SystemDark {
		systemDesc = "Automatically match your OS appearance (currently Dark)"
	}
	themes := [...]themeChoice{
		{"system", "System Default", systemDesc},
		{"dark", "macOS Dark", "Deep near-black dark theme"},
		{"light", "macOS Light", "Clean high-contrast light theme"},
		{"slate-dark", "Slate Dark", "GitHub-style slate dark palette"},
		{"slate-light", "Slate Light", "Soft slate light palette"},
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "APPEARANCE", unit.Sp(11), font.Bold, chrome.Colors.OnSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 4, Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, "Select the visual color scheme for the application chrome and components.", unit.Sp(12), font.Normal, chrome.Colors.OnSurfaceVariant, 2)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
				return c.layoutThemeChoice(gtx, themeChoiceInput{
					Choice: themes[0], SelectedTheme: currentTheme, Chrome: chrome, OnSelect: input.OnSetTheme,
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return chrome.Inset(gtx, layout.Inset{Right: 4, Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
								return c.layoutThemeChoice(gtx, themeChoiceInput{
									Choice: themes[1], SelectedTheme: currentTheme, Chrome: chrome, OnSelect: input.OnSetTheme,
								})
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return chrome.Inset(gtx, layout.Inset{Left: 4, Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
								return c.layoutThemeChoice(gtx, themeChoiceInput{
									Choice: themes[2], SelectedTheme: currentTheme, Chrome: chrome, OnSelect: input.OnSetTheme,
								})
							})
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return chrome.Inset(gtx, layout.Inset{Right: 4, Top: 2}, func(gtx layout.Context) layout.Dimensions {
								return c.layoutThemeChoice(gtx, themeChoiceInput{
									Choice: themes[3], SelectedTheme: currentTheme, Chrome: chrome, OnSelect: input.OnSetTheme,
								})
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return chrome.Inset(gtx, layout.Inset{Left: 4, Top: 2}, func(gtx layout.Context) layout.Dimensions {
								return c.layoutThemeChoice(gtx, themeChoiceInput{
									Choice: themes[4], SelectedTheme: currentTheme, Chrome: chrome, OnSelect: input.OnSetTheme,
								})
							})
						}),
					)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 16, Bottom: 16}, chrome.Divider)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, "ABOUT PROTONMAN DESKTOP", unit.Sp(11), font.Bold, chrome.Colors.OnSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 8}, func(gtx layout.Context) layout.Dimensions {
				return chrome.BorderSurface(gtx, unit.Dp(8), chrome.Colors.Surface, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 14, Bottom: 14, Left: 16, Right: 16}, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return chrome.Label(gtx, "Protonman Autonomous Coding Agent", unit.Sp(14), font.SemiBold, chrome.Colors.OnSurface, 1)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerHigh, func(gtx layout.Context) layout.Dimensions {
											return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 2, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
												return chrome.Label(gtx, buildinfo.Version(), unit.Sp(11), font.SemiBold, chrome.Colors.Primary, 1)
											})
										})
									}),
								)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, "Native Gio frontend driving the Protonman runtime over Agent Client Protocol (ACP) JSON-RPC stdio. Preserves clean architecture boundaries, fail-closed permission enforcement, and workspace confinement.", unit.Sp(12), font.Normal, chrome.Colors.OnSurfaceVariant, 4)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								tags := []string{"ACP stdio", "Clean Architecture", "Gio Native GUI", "Fail-Closed Security"}
								children := make([]layout.FlexChild, 0, len(tags))
								for i, tag := range tags {
									i, tag := i, tag
									children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return chrome.Inset(gtx, layout.Inset{Left: unit.Dp(i * 6)}, func(gtx layout.Context) layout.Dimensions { return c.layoutFeatureTag(gtx, tag, chrome) })
									}))
								}
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
							}),
						)
					})
				})
			})
		}),
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (c *Component) layoutFeatureTag(gtx layout.Context, label string, chrome Chrome) layout.Dimensions {
	return chrome.RoundedSurface(gtx, unit.Dp(4), chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
		return chrome.Inset(gtx, layout.Inset{Top: 3, Bottom: 3, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
			return chrome.Label(gtx, label, unit.Sp(11), font.Medium, chrome.Colors.OnSurfaceVariant, 1)
		})
	})
}

// themeChoice describes one selectable appearance option.
type themeChoice struct {
	mode  string
	label string
	desc  string
}

// themeChoiceInput groups the state needed to render one appearance option.
type themeChoiceInput struct {
	Choice        themeChoice
	SelectedTheme string
	Chrome        Chrome
	OnSelect      func(string)
}

func (c *Component) layoutThemeChoice(gtx layout.Context, in themeChoiceInput) layout.Dimensions {
	choice := in.Choice
	if c.themeButtons == nil {
		c.themeButtons = make(map[string]*widget.Clickable)
	}
	button := c.themeButtons[choice.mode]
	if button == nil {
		button = new(widget.Clickable)
		c.themeButtons[choice.mode] = button
	}
	chrome := in.Chrome
	selected := choice.mode == in.SelectedTheme
	if button.Clicked(gtx) && in.OnSelect != nil {
		in.OnSelect(choice.mode)
	}
	background, border, width := chrome.Colors.Surface, chrome.Colors.OutlineVariant, 1
	if selected {
		background, border, width = chrome.Colors.PrimaryContainer, chrome.Colors.Primary, 2
	} else if button.Hovered() {
		background = chrome.Colors.SurfaceContainerHigh
	}
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return chrome.BorderSurface(gtx, unit.Dp(8), background, border, width, func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						color := chrome.Colors.OnSurface
						if selected {
							color = chrome.Colors.OnPrimaryContainer
						}
						return chrome.Label(gtx, choice.label, unit.Sp(14), font.SemiBold, color, 1)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						color := chrome.Colors.OnSurfaceVariant
						if selected {
							color = chrome.Colors.OnPrimaryContainer
						}
						return chrome.Inset(gtx, layout.Inset{Top: 2}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, choice.desc, unit.Sp(11), font.Normal, color, 2)
						})
					}),
				)
			})
		})
	})
}

func (c *Component) escapePressed(gtx layout.Context) bool {
	for {
		event, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			return false
		}
		if ev, ok := event.(key.Event); ok && ev.State == key.Press {
			return true
		}
	}
}

func (c *Component) layoutHeader(gtx layout.Context, chrome Chrome) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return chrome.SettingsIcon(gtx, chrome.Colors.Primary) })
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Label(gtx, "Settings", unit.Sp(22), font.Bold, chrome.Colors.OnSurface, 1)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.CloseButton(gtx, &c.close, func() { c.Close() })
		}),
	)
}

func (c *Component) layoutTabs(gtx layout.Context, chrome Chrome) layout.Dimensions {
	return chrome.RoundedSurface(gtx, unit.Dp(12), chrome.Colors.SurfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 3, Bottom: 3, Left: 3, Right: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(tabLabels))
			for i, label := range tabLabels {
				i, label := i, label
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					button := &c.tabButtons[i]
					if button.Clicked(gtx) {
						c.activeTab = i
					}
					active := c.activeTab == i
					background, foreground, weight := color.NRGBA{}, chrome.Colors.OnSurfaceVariant, font.Medium
					if active {
						background, foreground, weight = chrome.Colors.Surface, chrome.Colors.OnSurface, font.SemiBold
					} else if button.Hovered() {
						background, foreground = chrome.Colors.SurfaceContainerHigh, chrome.Colors.OnSurface
					}
					gtx.Constraints.Min.Y = gtx.Dp(30)
					return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.RoundedSurface(gtx, unit.Dp(6), background, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Top: 5, Bottom: 5, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, label, unit.Sp(14), weight, foreground, 1)
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
