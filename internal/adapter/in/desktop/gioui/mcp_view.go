//go:build desktop || desktop_gio

package gioui

import (
	"encoding/json"
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
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
			summary := "No MCP integrations"
			if len(items) > 0 {
				names := make([]string, 0, len(items))
				for _, item := range items {
					names = append(names, item.Name)
				}
				summary = strings.Join(names, " · ")
			}
			return s.layoutLabel(gtx, compactInspectorText(summary, 240), textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 3)
		}),
	}

	if len(items) == 0 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Add a stdio server definition to make its tools available to new sessions.", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 4)
			})
		}))
	} else {
		for _, item := range items {
			item := item
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutMCPIntegrationRow(gtx, item, item.Name == s.mcpSelectedName, enabled)
			}))
		}
	}

	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		label := "Configure"
		if s.mcpFormVisible && s.mcpSelectedName != "" {
			label = "New integration"
		} else if s.mcpFormVisible {
			label = "Hide configuration"
		}
		return layout.UniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutButton(gtx, &s.mcpFormToggleButton, label, enabled, func() {
				if !s.mcpFormVisible {
					s.mcpFormVisible = true
					return
				}
				if s.mcpSelectedName != "" {
					s.mcpSelectedName = ""
					s.mcpEditorKey = ""
					s.clearMCPIntegrationEditors()
					return
				}
				s.mcpFormVisible = false
				s.clearMCPIntegrationEditors()
			})
		})
	}))

	if s.mcpFormVisible {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutMCPIntegrationEditor(gtx, "Name", &s.mcpNameEditor, enabled)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutMCPIntegrationEditor(gtx, "Command", &s.mcpCommandEditor, enabled)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutMCPIntegrationEditor(gtx, "Arguments JSON", &s.mcpArgsEditor, enabled)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutMCPIntegrationEditor(gtx, "Environment keys JSON", &s.mcpEnvEditor, enabled)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Values are read from the Protonman process environment and are never stored.", textLabelMedium, font.Normal, s.theme.onSurfaceVariant, 3)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &s.mcpSaveButton, "Save integration", enabled && strings.TrimSpace(s.mcpNameEditor.Text()) != "" && strings.TrimSpace(s.mcpCommandEditor.Text()) != "", func() {
					s.onSaveMCPIntegration(s.mcpNameEditor.Text(), s.mcpCommandEditor.Text(), s.mcpArgsEditor.Text(), s.mcpEnvEditor.Text())
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &s.mcpRemoveButton, "Remove integration", enabled && strings.TrimSpace(s.mcpSelectedName) != "", func() {
					s.onRemoveMCPIntegration(s.mcpSelectedName)
				})
			}),
		)
	}

	reconnectEnabled := anyAgentConnected(snapshot) && !snapshot.MCPUpdating && !snapshot.MCPReconnecting && !mcpAnySessionBusy(snapshot.State)
	reconnectLabel := "Reconnect ACP"
	if snapshot.MCPReconnecting {
		reconnectLabel = "Reconnecting…"
	}
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutButton(gtx, &s.mcpReconnectButton, reconnectLabel, reconnectEnabled, s.onReconnectMCP)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			status := "Reconnect ACP to apply changes to existing sessions"
			statusColor := s.theme.onSurfaceVariant
			if snapshot.MCPError != "" {
				status = "MCP settings unavailable · " + compactInspectorText(snapshot.MCPError, 240)
				statusColor = s.theme.onErrorContainer
			} else if snapshot.MCPUpdating {
				status = "Saving MCP settings…"
			} else if snapshot.MCPReconnecting {
				status = "Waiting for ACP to reconnect"
			} else if mcpAnySessionBusy(snapshot.State) {
				status = "Reconnect is unavailable while a session is active"
			}
			return s.layoutLabel(gtx, status, textLabelMedium, font.Normal, statusColor, 3)
		}),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) clearMCPIntegrationEditors() {
	s.mcpNameEditor.SetText("")
	s.mcpCommandEditor.SetText("")
	s.mcpArgsEditor.SetText("")
	s.mcpEnvEditor.SetText("")
}

func (s *shell) layoutMCPIntegrationRow(gtx layout.Context, item desktopstate.MCPIntegrationState, selected, enabled bool) layout.Dimensions {
	button := s.mcpIntegrationButtons[item.Name]
	if button == nil {
		button = new(widget.Clickable)
		s.mcpIntegrationButtons[item.Name] = button
	}
	gtx.Constraints.Min.Y = gtx.Dp(44)
	rowContext := gtx
	if !enabled {
		rowContext = gtx.Disabled()
	}
	if enabled && button.Clicked(rowContext) {
		s.mcpSelectedName = item.Name
		s.mcpEditorKey = ""
		s.mcpFormVisible = true
	}
	background := s.theme.surface
	foreground := s.theme.onSurface
	if selected {
		background = s.theme.secondaryContainer
		foreground = s.theme.onSecondaryContainer
	} else if button.Hovered() {
		background = s.theme.surfaceContainerHigh
	}
	dims := button.Layout(rowContext, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(44)
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		semantic.EnabledOp(gtx.Enabled()).Add(gtx.Ops)
		semantic.DescriptionOp("Edit MCP integration " + item.Name).Add(gtx.Ops)
		return s.roundedSurface(gtx, 12, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, item.Name, textBodyMedium, font.Medium, foreground, 1)
			})
		})
	})
	if enabled && rowContext.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: 12, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *shell) layoutMCPIntegrationEditor(gtx layout.Context, label string, editor *widget.Editor, enabled bool) layout.Dimensions {
	if enabled {
		for {
			if _, ok := editor.Update(gtx); !ok {
				break
			}
		}
	}
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
