//go:build desktop || desktop_gio

package gioui

import (
	"encoding/json"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func (s *shell) syncMCPIntegrationEditors(state desktopstate.State) {
	if s.syncRevision != 0 && s.mcpSyncRevisionSet && s.mcpSyncRevision == s.syncRevision {
		return
	}
	live := s.mcpIntegrationLive
	if live == nil {
		live = make(map[string]struct{}, len(state.Integrations))
		s.mcpIntegrationLive = live
	}
	clear(live)
	for _, item := range state.Integrations {
		live[item.Name] = struct{}{}
		if s.mcpIntegrationButtons[item.Name] == nil {
			s.mcpIntegrationButtons[item.Name] = new(widget.Clickable)
		}
	}
	for name := range s.mcpIntegrationButtons {
		if _, ok := live[name]; !ok {
			delete(s.mcpIntegrationButtons, name)
		}
	}

	if s.mcpSelectedName != "" {
		if _, ok := live[s.mcpSelectedName]; !ok {
			s.mcpSelectedName = ""
			s.mcpEditorKey = ""
			s.mcpFormVisible = false
			s.mcpConfirmDelete = false
			s.clearMCPIntegrationEditors()
		}
	}
	if s.mcpSelectedName == "" || s.mcpSelectedName == s.mcpEditorKey {
		if s.syncRevision != 0 {
			s.mcpSyncRevision = s.syncRevision
			s.mcpSyncRevisionSet = true
		}
		return
	}
	for _, item := range state.Integrations {
		if item.Name != s.mcpSelectedName {
			continue
		}
		s.mcpEditorKey = item.Name
		s.mcpNameEditor.SetText(item.Name)
		s.mcpCommandEditor.SetText(item.Command)
		s.mcpArgsEditor.SetText(mcpJSONList(item.Args))
		s.mcpEnvEditor.SetText(mcpJSONList(item.Env))
		if s.syncRevision != 0 {
			s.mcpSyncRevision = s.syncRevision
			s.mcpSyncRevisionSet = true
		}
		return
	}
	if s.syncRevision != 0 {
		s.mcpSyncRevision = s.syncRevision
		s.mcpSyncRevisionSet = true
	}
}

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

func (s *shell) layoutMCPIntegrationsPanel(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	items := snapshot.State.Integrations
	enabled := !snapshot.MCPUpdating && !snapshot.MCPReconnecting
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPanelTitle(gtx, "MCP integrations")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			summary := "No MCP integrations configured"
			if len(items) == 1 {
				summary = "1 active MCP integration · Select an integration card to edit"
			} else if len(items) > 1 {
				summary = strconv.Itoa(len(items)) + " active MCP integrations · Select an integration card to edit"
			}
			return desktopInset{Top: 2, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, summary, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 2)
			})
		}),
	}

	if len(items) == 0 && !s.mcpFormVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surface, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 14, Bottom: 14, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "No MCP integrations configured", textBodyMedium, font.SemiBold, s.theme.onSurface, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "Add a stdio server definition below to connect external tools like GitHub, Memory, or Filesystem to your Protonman sessions.", textBodySmall, font.Normal, s.theme.onSurfaceVariant, 3)
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
			return s.layoutMCPIntegrationRow(gtx, item, item.Name == s.mcpSelectedName && s.mcpFormVisible, enabled)
		}))
	}

	if enabled && s.mcpFormToggleButton.Clicked(gtx) {
		s.mcpFormVisible = true
		s.mcpSelectedName = ""
		s.mcpEditorKey = ""
		s.mcpConfirmDelete = false
		s.clearMCPIntegrationEditors()
		gtx.Execute(op.InvalidateCmd{})
	}

	if !s.mcpFormVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &s.mcpFormToggleButton, "+ Add MCP Integration", enabled, nil)
			})
		}))
	}

	if s.mcpFormVisible {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 10, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutMCPConfigurationCard(gtx, snapshot, items, enabled)
			})
		}))
	}

	reconnectEnabled := anyAgentConnected(snapshot) && !snapshot.MCPUpdating && !snapshot.MCPReconnecting && !mcpAnySessionBusy(snapshot.State)
	reconnectLabel := "Reconnect ACP"
	if snapshot.MCPReconnecting {
		reconnectLabel = "Reconnecting…"
	}

	status := "Reconnect ACP to apply changes to existing sessions. Environment values are read at launch and never stored."
	statusColor := s.theme.onSurfaceVariant
	if snapshot.MCPError != "" {
		status = "MCP settings unavailable · " + compactInspectorText(snapshot.MCPError, 240)
		statusColor = s.theme.onErrorContainer
	} else if snapshot.MCPUpdating {
		status = "Saving MCP settings…"
	} else if snapshot.MCPReconnecting {
		status = "Waiting for ACP to reconnect…"
	} else if mcpAnySessionBusy(snapshot.State) {
		status = "Reconnect is unavailable while a session is active"
	}

	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 12, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, status, textLabelSmall, font.Normal, statusColor, 3)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutButton(gtx, &s.mcpReconnectButton, reconnectLabel, reconnectEnabled, s.onReconnectMCP)
					}),
				)
			})
		}),
	)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) clearMCPIntegrationEditors() {
	s.mcpNameEditor.SetText("")
	s.mcpCommandEditor.SetText("")
	s.mcpArgsEditor.SetText("")
	s.mcpEnvEditor.SetText("")
	s.mcpConfirmDelete = false
}

func (s *shell) layoutMCPIntegrationRow(gtx layout.Context, item desktopstate.MCPIntegrationState, selected, enabled bool) layout.Dimensions {
	button := s.mcpIntegrationButtons[item.Name]
	if button == nil {
		button = new(widget.Clickable)
		s.mcpIntegrationButtons[item.Name] = button
	}
	rowContext := gtx
	if !enabled {
		rowContext = gtx.Disabled()
	}
	if enabled && button.Clicked(rowContext) {
		s.mcpSelectedName = item.Name
		s.mcpEditorKey = ""
		s.mcpFormVisible = true
		s.mcpConfirmDelete = false
		gtx.Execute(op.InvalidateCmd{})
	}
	gtx.Constraints.Min.Y = gtx.Dp(52)
	bg := s.theme.surface
	borderColor := s.theme.outlineVariant
	borderWidth := 1
	if selected {
		bg = s.theme.surfaceContainerHigh
		borderColor = s.theme.primary
		borderWidth = 2
	} else if button.Hovered() {
		bg = s.theme.surfaceContainerHigh
	}

	cmdText := item.Command
	if len(item.Args) > 0 {
		cmdText += " " + strings.Join(item.Args, " ")
	}

	dims := button.Layout(rowContext, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.roundedBorderSurface(gtx, shapeMedium, bg, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						// Header row: Status Dot + Name + Badge + Spacer + Edit hint
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutStatusDot(gtx, s.theme.primary)
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, item.Name, textBodyMedium, font.SemiBold, s.theme.onSurface, 1)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.roundedSurface(gtx, shapeSmall, s.theme.surfaceContainerLow, func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 2, Bottom: 2, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutLabel(gtx, "mcp stdio", textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
											})
										})
									})
								}),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Spacer{}.Layout(gtx)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "Edit ✎", textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
								}),
							)
						}),
						// Subtitle row: Command + Args
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 4, Left: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, cmdText, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 1)
							})
						}),
						// Optional Env tags
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if len(item.Env) == 0 {
								return layout.Dimensions{}
							}
							return desktopInset{Top: 4, Left: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "env: "+strings.Join(item.Env, ", "), textLabelSmall, font.Normal, s.theme.outline, 1)
							})
						}),
					)
				})
			})
		})
	})
	return dims
}

func (s *shell) layoutMCPConfigurationCard(gtx layout.Context, snapshot controllerSnapshot, items []desktopstate.MCPIntegrationState, enabled bool) layout.Dimensions {
	isEditing := s.mcpSelectedName != ""
	title := "Add New MCP Integration"
	if isEditing {
		title = "Edit MCP: " + s.mcpSelectedName
	}

	canRemove := enabled && isEditing
	canSave := enabled && strings.TrimSpace(s.mcpNameEditor.Text()) != "" && strings.TrimSpace(s.mcpCommandEditor.Text()) != ""

	if gtx.Constraints.Max.X > 0 {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
	}
	return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		if gtx.Constraints.Max.X > 0 {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
		}
		return desktopInset{Top: 16, Bottom: 16, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				// Header
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, title, textTitleMedium, font.Bold, s.theme.onSurface, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if s.mcpCloseButton.Clicked(gtx) {
								s.mcpFormVisible = false
								s.mcpSelectedName = ""
								s.mcpEditorKey = ""
								s.mcpConfirmDelete = false
								s.clearMCPIntegrationEditors()
								gtx.Execute(op.InvalidateCmd{})
							}
							return s.layoutMiniIconButton(gtx, &s.mcpCloseButton, "✕", s.theme.onSurfaceVariant)
						}),
					)
				}),
				// Presets bar (only shown when adding new integration)
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if isEditing {
						return layout.Dimensions{}
					}
					return desktopInset{Top: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutMCPPresetsBar(gtx, enabled)
					})
				}),
				// Form editors: Name and Command in 2-column layout
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 8, Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return s.layoutMCPIntegrationEditor(gtx, "Name", &s.mcpNameEditor, enabled)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{Width: 12}.Layout(gtx)
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return s.layoutMCPIntegrationEditor(gtx, "Command", &s.mcpCommandEditor, enabled)
							}),
						)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutMCPIntegrationEditor(gtx, "Arguments (flags or space-separated, e.g. -y @modelcontextprotocol/server-github)", &s.mcpArgsEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutMCPIntegrationEditor(gtx, "Environment keys (space or comma-separated, e.g. GITHUB_TOKEN)", &s.mcpEnvEditor, enabled)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "Environment values are read from the Protonman process at runtime and never persisted to disk.", textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 2)
					})
				}),
				// Actions / Delete Confirmation
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if s.mcpConfirmDelete {
						return desktopInset{Top: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.roundedBorderSurface(gtx, shapeSmall, s.theme.errorContainer, s.theme.errorContainer, 1, func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, "Delete this MCP integration? This cannot be undone.", textBodySmall, font.SemiBold, s.theme.onErrorContainer, 2)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Top: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return s.layoutButton(gtx, &s.mcpCancelDeleteBtn, "Cancel", enabled, func() {
															s.mcpConfirmDelete = false
														})
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return s.layoutDangerButton(gtx, &s.mcpConfirmDeleteBtn, "Confirm Delete", enabled, func() {
															targetName := s.mcpSelectedName
															s.mcpFormVisible = false
															s.mcpSelectedName = ""
															s.mcpEditorKey = ""
															s.mcpConfirmDelete = false
															s.clearMCPIntegrationEditors()
															s.onRemoveMCPIntegration(targetName)
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

					return desktopInset{Top: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !canRemove {
									return layout.Dimensions{}
								}
								return s.layoutDangerButton(gtx, &s.mcpRemoveButton, "Remove integration", canRemove, func() {
									s.mcpConfirmDelete = true
								})
							}),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Spacer{}.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutButton(gtx, &s.mcpCancelButton, "Cancel", enabled, func() {
												s.mcpFormVisible = false
												s.mcpSelectedName = ""
												s.mcpEditorKey = ""
												s.mcpConfirmDelete = false
												s.clearMCPIntegrationEditors()
												gtx.Execute(op.InvalidateCmd{})
											})
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutPrimaryButton(gtx, &s.mcpSaveButton, "Save integration", canSave, func() {
											name := s.mcpNameEditor.Text()
											cmd := s.mcpCommandEditor.Text()
											args := s.mcpArgsEditor.Text()
											env := s.mcpEnvEditor.Text()
											s.mcpFormVisible = false
											s.mcpSelectedName = ""
											s.mcpEditorKey = ""
											s.mcpConfirmDelete = false
											s.clearMCPIntegrationEditors()
											s.onSaveMCPIntegration(name, cmd, args, env)
										})
									}),
								)
							}),
						)
					})
				}),
			)
		})
	})
}

func (s *shell) layoutMCPPresetsBar(gtx layout.Context, enabled bool) layout.Dimensions {
	if s.mcpPresetGitHubBtn.Clicked(gtx) {
		s.mcpNameEditor.SetText("github")
		s.mcpCommandEditor.SetText("npx")
		s.mcpArgsEditor.SetText("-y @modelcontextprotocol/server-github")
		s.mcpEnvEditor.SetText("GITHUB_PERSONAL_ACCESS_TOKEN")
	}
	if s.mcpPresetMemoryBtn.Clicked(gtx) {
		s.mcpNameEditor.SetText("memory")
		s.mcpCommandEditor.SetText("npx")
		s.mcpArgsEditor.SetText("-y @modelcontextprotocol/server-memory")
		s.mcpEnvEditor.SetText("")
	}
	if s.mcpPresetFilesystemBtn.Clicked(gtx) {
		s.mcpNameEditor.SetText("filesystem")
		s.mcpCommandEditor.SetText("npx")
		s.mcpArgsEditor.SetText("-y @modelcontextprotocol/server-filesystem .")
		s.mcpEnvEditor.SetText("")
	}
	if s.mcpPresetFetchBtn.Clicked(gtx) {
		s.mcpNameEditor.SetText("fetch")
		s.mcpCommandEditor.SetText("uvx")
		s.mcpArgsEditor.SetText("mcp-server-fetch")
		s.mcpEnvEditor.SetText("")
	}
	if s.mcpPresetCustomBtn.Clicked(gtx) {
		s.clearMCPIntegrationEditors()
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, "QUICK PRESETS", textLabelSmall, font.Bold, s.theme.onSurfaceVariant, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutPresetChip(gtx, &s.mcpPresetGitHubBtn, "GitHub", enabled)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutPresetChip(gtx, &s.mcpPresetMemoryBtn, "Memory", enabled)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutPresetChip(gtx, &s.mcpPresetFilesystemBtn, "Filesystem", enabled)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutPresetChip(gtx, &s.mcpPresetFetchBtn, "Fetch", enabled)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutPresetChip(gtx, &s.mcpPresetCustomBtn, "Custom", enabled)
						})
					}),
				)
			})
		}),
	)
}

func (s *shell) layoutMCPIntegrationEditor(gtx layout.Context, label string, editor *widget.Editor, enabled bool) layout.Dimensions {
	return s.layoutInspectorEditor(gtx, label, editor, enabled)
}

func mcpAnySessionBusy(state desktopstate.State) bool {
	for _, session := range state.Sessions {
		if sessionBusy(session.Status) {
			return true
		}
	}
	return false
}
