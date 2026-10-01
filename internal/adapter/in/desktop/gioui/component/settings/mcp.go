//go:build desktop || desktop_gio

package settings

import (
	"encoding/json"
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

type MCPInput struct {
	Integrations           []MCPIntegration
	Updating, Reconnecting bool
	Connected, SessionBusy bool
	Error                  string
	Chrome                 Chrome
	OnSave                 func(name, command, args, env string)
	OnRemove               func(name string)
	OnReconnect            func()
}

// SyncMCPIntegrationEditors applies new integration snapshots without replacing
// a draft the user is currently editing.
func (c *Component) SyncMCPIntegrationEditors(revision uint64, items []MCPIntegration) {
	mcp := &c.mcp
	if revision != 0 && mcp.SyncRevisionSet && mcp.SyncRevision == revision {
		return
	}
	if mcp.IntegrationLive == nil {
		mcp.IntegrationLive = make(map[string]struct{}, len(items))
	}
	clear(mcp.IntegrationLive)
	for _, item := range items {
		mcp.IntegrationLive[item.Name] = struct{}{}
		if mcp.IntegrationButtons[item.Name] == nil {
			mcp.IntegrationButtons[item.Name] = newClickable()
		}
	}
	for name := range mcp.IntegrationButtons {
		if _, ok := mcp.IntegrationLive[name]; !ok {
			delete(mcp.IntegrationButtons, name)
		}
	}
	if mcp.SelectedName != "" {
		if _, ok := mcp.IntegrationLive[mcp.SelectedName]; !ok {
			mcp.SelectedName, mcp.EditorKey, mcp.FormVisible, mcp.ConfirmDelete = "", "", false, false
			c.clearMCPEditors()
		}
	}
	if mcp.SelectedName == "" || mcp.SelectedName == mcp.EditorKey {
		c.setMCPSyncRevision(revision)
		return
	}
	for _, item := range items {
		if item.Name != mcp.SelectedName {
			continue
		}
		mcp.EditorKey = item.Name
		mcp.NameEditor.SetText(item.Name)
		mcp.CommandEditor.SetText(item.Command)
		mcp.ArgsEditor.SetText(mcpJSONList(item.Args))
		mcp.EnvEditor.SetText(mcpJSONList(item.Env))
		c.setMCPSyncRevision(revision)
		return
	}
	c.setMCPSyncRevision(revision)
}

func newClickable() *widget.Clickable { return new(widget.Clickable) }

func (c *Component) setMCPSyncRevision(revision uint64) {
	if revision != 0 {
		c.mcp.SyncRevision, c.mcp.SyncRevisionSet = revision, true
	}
}

func (c *Component) clearMCPEditors() {
	c.mcp.NameEditor.SetText("")
	c.mcp.CommandEditor.SetText("")
	c.mcp.ArgsEditor.SetText("")
	c.mcp.EnvEditor.SetText("")
	c.mcp.ConfirmDelete = false
}

func (c *Component) ClearMCPIntegrationEditors() { c.clearMCPEditors() }

func mcpJSONList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

// LayoutMCP owns the MCP settings view, form state, and destructive-action flow.
func (c *Component) LayoutMCP(gtx layout.Context, input MCPInput) layout.Dimensions {
	mcp, chrome := &c.mcp, input.Chrome
	items := input.Integrations
	enabled := !input.Updating && !input.Reconnecting
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return chrome.PanelTitle(gtx, "MCP integrations") }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			summary := "No MCP integrations configured"
			if len(items) == 1 {
				summary = "1 active MCP integration · Select an integration card to edit"
			} else if len(items) > 1 {
				summary = fmt.Sprintf("%d active MCP integrations · Select an integration card to edit", len(items))
			}
			return chrome.Inset(gtx, layout.Inset{Top: 2, Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Label(gtx, summary, unit.Sp(12), font.Normal, chrome.Colors.OnSurfaceVariant, 2)
			})
		}),
	}
	if len(items) == 0 && !mcp.FormVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
				return chrome.BorderSurface(gtx, unit.Dp(8), chrome.Colors.Surface, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 14, Bottom: 14, Left: 16, Right: 16}, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return chrome.Label(gtx, "No MCP integrations configured", unit.Sp(14), font.SemiBold, chrome.Colors.OnSurface, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return chrome.Inset(gtx, layout.Inset{Top: 4}, func(gtx layout.Context) layout.Dimensions {
									return chrome.Label(gtx, "Add a stdio server definition below to connect external tools like GitHub, Memory, or Filesystem to your Protonman sessions.", unit.Sp(12), font.Normal, chrome.Colors.OnSurfaceVariant, 3)
								})
							}),
						)
					})
				})
			})
		}))
	}
	for _, item := range items {
		item := item
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.LayoutMCPIntegrationRow(gtx, item, item.Name == mcp.SelectedName && mcp.FormVisible, enabled, chrome)
		}))
	}
	if enabled && mcp.FormToggleButton.Clicked(gtx) {
		mcp.FormVisible, mcp.SelectedName, mcp.EditorKey, mcp.ConfirmDelete = true, "", "", false
		c.clearMCPEditors()
		gtx.Execute(op.InvalidateCmd{})
	}
	if !mcp.FormVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 6, Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
				return chrome.Button(gtx, &mcp.FormToggleButton, "+ Add MCP Integration", enabled, nil)
			})
		}))
	}
	if mcp.FormVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return chrome.Inset(gtx, layout.Inset{Top: 10, Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
				return c.layoutMCPConfigurationCard(gtx, input, enabled)
			})
		}))
	}
	reconnectEnabled := input.Connected && !input.Updating && !input.Reconnecting && !input.SessionBusy
	reconnectLabel := "Reconnect ACP"
	if input.Reconnecting {
		reconnectLabel = "Reconnecting…"
	}
	status, statusColor := "Reconnect ACP to apply changes to existing sessions. Environment values are read at launch and never stored.", chrome.Colors.OnSurfaceVariant
	switch {
	case input.Error != "":
		status, statusColor = "MCP settings unavailable · "+compactMCPError(input.Error, 240), chrome.Colors.OnErrorContainer
	case input.Updating:
		status = "Saving MCP settings…"
	case input.Reconnecting:
		status = "Waiting for ACP to reconnect…"
	case input.SessionBusy:
		status = "Reconnect is unavailable while a session is active"
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return chrome.Inset(gtx, layout.Inset{Top: 12, Bottom: 8}, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Right: 12}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, status, unit.Sp(11), font.Normal, statusColor, 3)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Button(gtx, &mcp.ReconnectButton, reconnectLabel, reconnectEnabled, input.OnReconnect)
				}),
			)
		})
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (c *Component) layoutMCPConfigurationCard(gtx layout.Context, input MCPInput, enabled bool) layout.Dimensions {
	mcp, chrome := &c.mcp, input.Chrome
	isEditing := mcp.SelectedName != ""
	title := "Add New MCP Integration"
	if isEditing {
		title = "Edit MCP: " + mcp.SelectedName
	}
	canRemove := enabled && isEditing
	canSave := enabled && strings.TrimSpace(mcp.NameEditor.Text()) != "" && strings.TrimSpace(mcp.CommandEditor.Text()) != ""
	if gtx.Constraints.Max.X > 0 {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
	}
	return chrome.BorderSurface(gtx, unit.Dp(8), chrome.Colors.SurfaceContainerHigh, chrome.Colors.OutlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		if gtx.Constraints.Max.X > 0 {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
		}
		return chrome.Inset(gtx, layout.Inset{Top: 16, Bottom: 16, Left: 16, Right: 16}, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, title, unit.Sp(16), font.Bold, chrome.Colors.OnSurface, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if mcp.CloseButton.Clicked(gtx) {
								mcp.FormVisible, mcp.SelectedName, mcp.EditorKey, mcp.ConfirmDelete = false, "", "", false
								c.clearMCPEditors()
								gtx.Execute(op.InvalidateCmd{})
							}
							return chrome.MiniIconButton(gtx, &mcp.CloseButton, "✕", chrome.Colors.OnSurfaceVariant)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if isEditing {
						return layout.Dimensions{}
					}
					return chrome.Inset(gtx, layout.Inset{Top: 10}, func(gtx layout.Context) layout.Dimensions { return c.LayoutMCPPresets(gtx, enabled, chrome) })
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Top: 8, Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return chrome.Editor(gtx, "Name", &mcp.NameEditor, enabled)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Width: unit.Dp(12)}.Layout(gtx) }),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return chrome.Editor(gtx, "Command", &mcp.CommandEditor, enabled)
							}),
						)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 10}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Editor(gtx, "Arguments (flags or space-separated, e.g. -y @modelcontextprotocol/server-github)", &mcp.ArgsEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 6}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Editor(gtx, "Environment keys (space or comma-separated, e.g. GITHUB_TOKEN)", &mcp.EnvEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return chrome.Inset(gtx, layout.Inset{Bottom: 4}, func(gtx layout.Context) layout.Dimensions {
						return chrome.Label(gtx, "Environment values are read from the Protonman process at runtime and never persisted to disk.", unit.Sp(11), font.Normal, chrome.Colors.OnSurfaceVariant, 2)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.layoutMCPActions(gtx, input, enabled, canRemove, canSave)
				}),
			)
		})
	})
}

func (c *Component) layoutMCPActions(gtx layout.Context, input MCPInput, enabled, canRemove, canSave bool) layout.Dimensions {
	mcp, chrome := &c.mcp, input.Chrome
	if mcp.ConfirmDelete {
		return chrome.Inset(gtx, layout.Inset{Top: 12}, func(gtx layout.Context) layout.Dimensions {
			return chrome.BorderSurface(gtx, unit.Dp(4), chrome.Colors.ErrorContainer, chrome.Colors.ErrorContainer, 1, func(gtx layout.Context) layout.Dimensions {
				return chrome.Inset(gtx, layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return chrome.Label(gtx, "Delete this MCP integration? This cannot be undone.", unit.Sp(12), font.SemiBold, chrome.Colors.OnErrorContainer, 2)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return chrome.Inset(gtx, layout.Inset{Top: 8}, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return chrome.Button(gtx, &mcp.CancelDeleteButton, "Cancel", enabled, func() { mcp.ConfirmDelete = false })
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return chrome.DangerButton(gtx, &mcp.ConfirmDeleteButton, "Confirm Delete", enabled, func() {
											target := mcp.SelectedName
											c.closeMCPForm()
											if input.OnRemove != nil {
												input.OnRemove(target)
											}
										})
									}),
								)
							})
						}),
					)
				})
			})
		})
	}
	return chrome.Inset(gtx, layout.Inset{Top: 14}, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !canRemove {
					return layout.Dimensions{}
				}
				return chrome.DangerButton(gtx, &mcp.RemoveButton, "Remove integration", canRemove, func() { mcp.ConfirmDelete = true })
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Spacer{}.Layout(gtx) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.Inset(gtx, layout.Inset{Right: 8}, func(gtx layout.Context) layout.Dimensions {
							return chrome.Button(gtx, &mcp.CancelButton, "Cancel", enabled, func() { c.closeMCPForm(); gtx.Execute(op.InvalidateCmd{}) })
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return chrome.PrimaryButton(gtx, &mcp.SaveButton, "Save integration", canSave, func() {
							name, command, args, env := mcp.NameEditor.Text(), mcp.CommandEditor.Text(), mcp.ArgsEditor.Text(), mcp.EnvEditor.Text()
							c.closeMCPForm()
							if input.OnSave != nil {
								input.OnSave(name, command, args, env)
							}
						})
					}),
				)
			}),
		)
	})
}

func (c *Component) closeMCPForm() {
	c.mcp.FormVisible, c.mcp.SelectedName, c.mcp.EditorKey, c.mcp.ConfirmDelete = false, "", "", false
	c.clearMCPEditors()
}

func compactMCPError(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes-1]) + "…"
}
