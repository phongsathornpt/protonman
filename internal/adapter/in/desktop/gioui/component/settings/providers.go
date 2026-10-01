//go:build desktop || desktop_gio

package settings

import (
	"fmt"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

type ProviderSaveParams struct {
	ProviderName string
	ProviderType string
	PreviousName string
	BaseURL      string
	APIKey       string
	DefaultModel string
	Activate     bool
}

type ProviderFormInput struct {
	SelectedID            string
	ProviderModels        map[string][]string
	ProviderModelsLoading map[string]bool
	Updating              bool
	Chrome                Chrome
	OnSave                func(ProviderSaveParams, func(error))
	OnDelete              func(string, func(error))
	OnFetchModels         func(id, endpoint, key, proto string, cb func([]string, error))
}

func (c *Component) OpenProviderForm(id, name, endpoint, protocol, defaultModel string) {
	c.provider.FormVisible = true
	c.provider.FormSelectedID = id
	c.provider.NameEditor.SetText(name)
	c.provider.EndpointEditor.SetText(endpoint)
	c.provider.APIKeyEditor.SetText("")
	c.provider.SelectedType = protocol
	if c.provider.SelectedType == "" {
		c.provider.SelectedType = "openai"
	}
	c.provider.ShowKey = false
	c.provider.TestStatus = ""
	c.provider.TestError = ""
	c.provider.DiscoveredModels = nil
	c.provider.SelectedModel = defaultModel
	c.provider.ModelsDropdownOpen = false
}

func (c *Component) CloseProviderForm() {
	c.provider.FormVisible = false
	c.provider.FormSelectedID = ""
}

func (c *Component) ProviderModelSelectButton(id string) *widget.Clickable {
	btn, ok := c.provider.ModelSelectButtons[id]
	if !ok {
		btn = new(widget.Clickable)
		if c.provider.ModelSelectButtons == nil {
			c.provider.ModelSelectButtons = make(map[string]*widget.Clickable)
		}
		c.provider.ModelSelectButtons[id] = btn
	}
	return btn
}

func (c *Component) LayoutProviderForm(gtx layout.Context, input ProviderFormInput) layout.Dimensions {
	isPreset := false
	for _, presetID := range []string{"opencode", "opencode-zen", "opencode-go", "protonman", "ollama", "openai", "anthropic"} {
		if strings.EqualFold(presetID, c.provider.FormSelectedID) {
			isPreset = true
			break
		}
	}

	title := "Configure " + c.provider.FormSelectedID
	if !isPreset && c.provider.FormSelectedID == "" {
		title = "Add Custom Provider"
	}

	chrome := input.Chrome
	enabled := !input.Updating

	return chrome.BorderSurface(gtx, unit.Dp(8), chrome.Colors.SurfaceContainerLow, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		return chrome.Inset(gtx, layout.Inset{Top: 12, Bottom: 12, Left: 14, Right: 14}, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Label(gtx, title, unit.Sp(16), font.SemiBold, chrome.Colors.OnSurface, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
						nameEditable := !isPreset && enabled
						return c.layoutProviderFieldEditor(gtx, "Provider Name", &c.provider.NameEditor, nameEditable, chrome)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
						return c.layoutProviderFieldEditor(gtx, "Base URL / Endpoint", &c.provider.EndpointEditor, enabled, chrome)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
						return c.layoutProtocolSelector(gtx, enabled, chrome)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
						return c.layoutProviderAPIKeyField(gtx, enabled, chrome)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 4, Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
						return c.layoutTestConnectionRow(gtx, testConnectionInput{
							Loading: input.ProviderModelsLoading, Enabled: enabled, Chrome: chrome, OnFetch: input.OnFetchModels,
						})
					})
				}),
			}

			models := c.provider.DiscoveredModels
			if len(models) == 0 && input.ProviderModels != nil {
				models = input.ProviderModels[strings.ToLower(c.provider.FormSelectedID)]
			}
			if len(models) > 0 {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
						return c.layoutModelSelectionDropdown(gtx, models, enabled, chrome)
					})
				}))
			}

			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return chrome.Inset(gtx, layout.Inset{Top: 6}, func(gtx layout.Context) layout.Dimensions {
					return c.layoutProviderFormButtons(gtx, providerFormInput{
						IsPreset: isPreset, Enabled: enabled, Chrome: chrome, OnSave: input.OnSave, OnDelete: input.OnDelete,
					})
				})
			}))

			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	})
}

func (c *Component) layoutProtocolSelector(gtx layout.Context, enabled bool, chrome Chrome) layout.Dimensions {
	isOpenAI := strings.ToLower(c.provider.SelectedType) != "anthropic"

	if enabled && c.provider.TypeOpenAIButton.Clicked(gtx) {
		c.provider.SelectedType = "openai"
	}
	if enabled && c.provider.TypeAnthropicButton.Clicked(gtx) {
		c.provider.SelectedType = "anthropic"
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Right: 12}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, "Protocol:", unit.Sp(12), font.Medium, chrome.Colors.OnSurfaceVariant, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bg := chrome.Colors.SurfaceContainerLow
			fg := chrome.Colors.OnSurfaceVariant
			if isOpenAI {
				bg = chrome.Colors.PrimaryContainer
				fg = chrome.Colors.OnPrimaryContainer
			}
			return c.provider.TypeOpenAIButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return chrome.RoundedSurface(gtx, unit.Dp(6), bg, func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 4, Bottom: 4, Left: 10, Right: 10}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, "OpenAI-compatible", unit.Sp(11), font.SemiBold, fg, 1)
					})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bg := chrome.Colors.SurfaceContainerLow
			fg := chrome.Colors.OnSurfaceVariant
			if !isOpenAI {
				bg = chrome.Colors.PrimaryContainer
				fg = chrome.Colors.OnPrimaryContainer
			}
			return chrome.Inset(gtx, layout.Inset{Left: 6}, func(gtx layout.Context) layout.Dimensions {
				return c.provider.TypeAnthropicButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return chrome.RoundedSurface(gtx, unit.Dp(6), bg, func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Top: 4, Bottom: 4, Left: 10, Right: 10}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, "Anthropic Messages", unit.Sp(11), font.SemiBold, fg, 1)
						})
					})
				})
			})
		}),
	)
}

func (c *Component) layoutProviderFieldEditor(gtx layout.Context, label string, ed *widget.Editor, enabled bool, chrome Chrome) layout.Dimensions {
	ed.ReadOnly = !enabled
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Bottom: 3}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, label, unit.Sp(11), font.Medium, chrome.Colors.OnSurfaceVariant, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.BorderSurface(gtx, unit.Dp(6), chrome.Colors.Surface, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
				return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}, func(gtx layout.Context) layout.Dimensions {
					mEd := material.Editor(chrome.Material, ed, "")
					mEd.TextSize = unit.Sp(12)
					mEd.Color = chrome.Colors.OnSurface
					return mEd.Layout(gtx)
				})
			})
		}),
	)
}

func (c *Component) layoutProviderAPIKeyField(gtx layout.Context, enabled bool, chrome Chrome) layout.Dimensions {
	if enabled && c.provider.ShowKeyButton.Clicked(gtx) {
		c.provider.ShowKey = !c.provider.ShowKey
	}

	maskChar := rune(0)
	if !c.provider.ShowKey {
		maskChar = '•'
	}
	c.provider.APIKeyEditor.Mask = maskChar
	c.provider.APIKeyEditor.ReadOnly = !enabled

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 3}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, "API Key", unit.Sp(11), font.Medium, chrome.Colors.OnSurfaceVariant, 1)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					toggleText := "Show key"
					if c.provider.ShowKey {
						toggleText = "Hide key"
					}
					return c.provider.ShowKeyButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, toggleText, unit.Sp(11), font.Normal, chrome.Colors.Primary, 1)
					})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.BorderSurface(gtx, unit.Dp(6), chrome.Colors.Surface, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
				return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}, func(gtx layout.Context) layout.Dimensions {
					mEd := material.Editor(chrome.Material, &c.provider.APIKeyEditor, "Leave blank to keep existing key")
					mEd.TextSize = unit.Sp(12)
					mEd.Color = chrome.Colors.OnSurface
					return mEd.Layout(gtx)
				})
			})
		}),
	)
}

// testConnectionInput groups the connection-test row state.
type testConnectionInput struct {
	Loading map[string]bool
	Enabled bool
	Chrome  Chrome
	OnFetch func(id, endpoint, key, proto string, cb func([]string, error))
}

func (c *Component) layoutTestConnectionRow(gtx layout.Context, in testConnectionInput) layout.Dimensions {
	loading := in.Loading
	enabled := in.Enabled
	chrome := in.Chrome
	onFetch := in.OnFetch
	id := c.provider.FormSelectedID
	if id == "" {
		id = strings.TrimSpace(c.provider.NameEditor.Text())
	}
	isLoading := loading != nil && loading[id]

	if enabled && !isLoading && c.provider.TestButton.Clicked(gtx) {
		c.provider.TestStatus = "Connecting & discovering models…"
		c.provider.TestError = ""
		endpoint := strings.TrimSpace(c.provider.EndpointEditor.Text())
		key := strings.TrimSpace(c.provider.APIKeyEditor.Text())
		proto := c.provider.SelectedType
		if onFetch != nil {
			onFetch(id, endpoint, key, proto, func(models []string, err error) {
				if err != nil {
					c.provider.TestError = err.Error()
					c.provider.TestStatus = ""
				} else {
					c.provider.DiscoveredModels = models
					c.provider.TestStatus = fmt.Sprintf("✓ Connected · %d models found", len(models))
					if len(models) > 0 && c.provider.SelectedModel == "" {
						c.provider.SelectedModel = models[0]
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
			return chrome.Button(gtx, &c.provider.TestButton, btnText, enabled && !isLoading, nil)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			statusText := c.provider.TestStatus
			fg := chrome.Colors.Primary
			if c.provider.TestError != "" {
				statusText = "✗ " + c.provider.TestError
				fg = chrome.Colors.OnErrorContainer
			}
			if statusText == "" {
				return layout.Dimensions{}
			}
			return chrome.Inset(gtx, layout.Inset{Left: 10}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, statusText, unit.Sp(11), font.Medium, fg, 2)
			})
		}),
	)
}

func (c *Component) layoutModelSelectionDropdown(gtx layout.Context, models []string, enabled bool, chrome Chrome) layout.Dimensions {
	if enabled && c.provider.ModelsDropdownButton.Clicked(gtx) {
		c.provider.ModelsDropdownOpen = !c.provider.ModelsDropdownOpen
	}

	current := c.provider.SelectedModel
	if current == "" && len(models) > 0 {
		current = models[0]
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Bottom: 3}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, "Default Model", unit.Sp(11), font.Medium, chrome.Colors.OnSurfaceVariant, 1)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.provider.ModelsDropdownButton.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return chrome.BorderSurface(gtx, unit.Dp(6), chrome.Colors.Surface, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, current, unit.Sp(12), font.Medium, chrome.Colors.OnSurface, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								icon := uikit.IconChevronDown
								if c.provider.ModelsDropdownOpen {
									icon = uikit.IconChevronUp
								}
								return chrome.ActionIcon(gtx, icon, unit.Dp(12), chrome.Colors.OnSurfaceVariant)
							}),
						)
					})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !c.provider.ModelsDropdownOpen {
				return layout.Dimensions{}
			}
			rows := make([]layout.FlexChild, 0, min(8, len(models)))
			for _, m := range models {
				modelID := m
				btn := c.ProviderModelSelectButton(modelID)
				if enabled && btn.Clicked(gtx) {
					c.provider.SelectedModel = modelID
					c.provider.ModelsDropdownOpen = false
				}
				isSelected := modelID == current
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					bg := color.NRGBA{}
					fg := chrome.Colors.OnSurface
					if isSelected {
						bg = chrome.Colors.PrimaryContainer
						fg = chrome.Colors.OnPrimaryContainer
					}
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return chrome.RoundedSurface(gtx, unit.Dp(6), bg, func(gtx layout.Context) layout.Dimensions {
							return chrome.Inset(gtx, layout.Inset{Top: 4, Bottom: 4, Left: 8, Right: 8}, func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, modelID, unit.Sp(12), font.Normal, fg, 1)
							})
						})
					})
				}))
			}
			return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
				return chrome.BorderSurface(gtx, unit.Dp(6), chrome.Colors.SurfaceContainerHigh, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 4, Bottom: 4, Left: 4, Right: 4}, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
					})
				})
			})
		}),
	)
}

// providerFormInput groups the provider form's save/delete action row.
type providerFormInput struct {
	IsPreset bool
	Enabled  bool
	Chrome   Chrome
	OnSave   func(ProviderSaveParams, func(error))
	OnDelete func(string, func(error))
}

func (c *Component) layoutProviderFormButtons(gtx layout.Context, in providerFormInput) layout.Dimensions {
	isPreset := in.IsPreset
	enabled := in.Enabled
	chrome := in.Chrome
	onSave := in.OnSave
	onDelete := in.OnDelete
	providerName := strings.TrimSpace(c.provider.NameEditor.Text())
	if providerName == "" {
		providerName = c.provider.FormSelectedID
	}

	canSave := providerName != "" && enabled

	if canSave && c.provider.SaveActivateButton.Clicked(gtx) && onSave != nil {
		c.submitProviderSave(providerName, true, onSave)
	}
	if canSave && c.provider.SaveButton.Clicked(gtx) && onSave != nil {
		c.submitProviderSave(providerName, false, onSave)
	}
	if enabled && c.provider.CancelButton.Clicked(gtx) {
		c.CloseProviderForm()
	}
	if enabled && c.provider.DeleteButton.Clicked(gtx) && onDelete != nil {
		onDelete(providerName, func(err error) {
			if err == nil {
				c.CloseProviderForm()
			}
		})
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.PrimaryButton(gtx, &c.provider.SaveActivateButton, "Save & Activate", canSave, nil)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Button(gtx, &c.provider.SaveButton, "Save", canSave, nil)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Left: 8}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Button(gtx, &c.provider.CancelButton, "Cancel", enabled, nil)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Spacer{}.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !isPreset && c.provider.FormSelectedID != "" {
				return chrome.DangerButton(gtx, &c.provider.DeleteButton, "Delete Provider", enabled, nil)
			}
			return layout.Dimensions{}
		}),
	)
}

func (c *Component) submitProviderSave(name string, activate bool, onSave func(ProviderSaveParams, func(error))) {
	params := ProviderSaveParams{
		ProviderName: name,
		ProviderType: c.provider.SelectedType,
		PreviousName: c.provider.FormSelectedID,
		BaseURL:      strings.TrimSpace(c.provider.EndpointEditor.Text()),
		APIKey:       strings.TrimSpace(c.provider.APIKeyEditor.Text()),
		DefaultModel: strings.TrimSpace(c.provider.SelectedModel),
		Activate:     activate,
	}
	onSave(params, func(err error) {
		if err == nil {
			c.CloseProviderForm()
		}
	})
}
