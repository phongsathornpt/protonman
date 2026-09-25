//go:build desktop || desktop_gio

package gioui

import (
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/phongsathornpt/protonman/internal/app"
)

func activeAgentDisplayName(snapshot controllerSnapshot) string {
	return agentDisplayName(snapshot.AgentProfiles, snapshot.ActiveAgentID)
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

func (s *shell) syncAgentProfileEditors(snapshot controllerSnapshot) {
	if s.syncRevision != 0 && s.agentSyncRevisionSet && s.agentSyncRevision == s.syncRevision {
		return
	}
	live := s.agentProfileLive
	if live == nil {
		live = make(map[string]struct{}, len(snapshot.AgentProfiles))
		s.agentProfileLive = live
	}
	clear(live)
	for _, profile := range snapshot.AgentProfiles {
		live[profile.ID] = struct{}{}
		if s.agentProfileButtons[profile.ID] == nil {
			s.agentProfileButtons[profile.ID] = new(widget.Clickable)
		}
		if s.agentChoiceButtons[profile.ID] == nil {
			s.agentChoiceButtons[profile.ID] = new(widget.Clickable)
		}
	}
	for agentID := range s.agentProfileButtons {
		if _, ok := live[agentID]; !ok {
			delete(s.agentProfileButtons, agentID)
		}
	}
	for agentID := range s.agentChoiceButtons {
		if _, ok := live[agentID]; !ok {
			delete(s.agentChoiceButtons, agentID)
		}
	}

	if s.agentEditorOriginalID != "" {
		if _, ok := live[s.agentEditorOriginalID]; !ok {
			s.agentEditorOriginalID = ""
			s.agentEditorKey = ""
			s.agentEditorVisible = false
			s.clearAgentProfileEditors()
		}
	}
	if !s.agentEditorVisible || s.agentEditorOriginalID == "" || s.agentEditorOriginalID == s.agentEditorKey {
		if s.syncRevision != 0 {
			s.agentSyncRevision = s.syncRevision
			s.agentSyncRevisionSet = true
		}
		return
	}
	for _, profile := range snapshot.AgentProfiles {
		if profile.ID != s.agentEditorOriginalID {
			continue
		}
		s.agentEditorKey = profile.ID
		s.agentIDEditor.SetText(profile.ID)
		s.agentNameEditor.SetText(profile.DisplayName)
		s.agentCommandEditor.SetText(profile.Command)
		s.agentArgsEditor.SetText(mcpJSONList(profile.Args))
		s.agentEnvEditor.SetText(mcpJSONList(profile.Env))
		if s.syncRevision != 0 {
			s.agentSyncRevision = s.syncRevision
			s.agentSyncRevisionSet = true
		}
		return
	}
	if s.syncRevision != 0 {
		s.agentSyncRevision = s.syncRevision
		s.agentSyncRevisionSet = true
	}
}

func (s *shell) layoutAgentSelectorBar(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(56)
	return s.roundedSurface(gtx, 0, s.theme.surfaceContainer, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 6, Bottom: 6, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.agentSelectorList.Layout(gtx, len(snapshot.AgentProfiles), func(gtx layout.Context, index int) layout.Dimensions {
				profile := snapshot.AgentProfiles[index]
				return layout.UniformInset(3).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutAgentChoiceButton(gtx, profile, profile.ID == snapshot.ActiveAgentID, snapshot.AgentConnections[profile.ID])
				})
			})
		})
	})
}

func (s *shell) layoutAgentChoiceButton(gtx layout.Context, profile app.ACPAgentProfile, selected bool, phase connectionPhase) layout.Dimensions {
	button := s.agentChoiceButtons[profile.ID]
	if button.Clicked(gtx) {
		s.onSelectAgent(profile.ID)
		s.agentSelectorVisible = false
	}
	background := s.theme.surface
	foreground := s.theme.onSurface
	if selected {
		background = s.theme.secondaryContainer
		foreground = s.theme.onSecondaryContainer
	} else if button.Hovered() {
		background = s.theme.primaryContainer
		foreground = s.theme.onPrimaryContainer
	}
	gtx.Constraints.Min.X = gtx.Dp(148)
	gtx.Constraints.Min.Y = gtx.Dp(44)
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(44)
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		semantic.DescriptionOp("Select agent " + profile.DisplayName + ", " + connectionLabel(phase)).Add(gtx.Ops)
		return s.roundedSurface(gtx, 12, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, profile.DisplayName+" · "+connectionLabel(phase), textLabelMedium, font.SemiBold, foreground, 1)
			})
		})
	})
	if gtx.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: 12, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func connectionLabel(phase connectionPhase) string {
	switch phase {
	case connectionConnected:
		return "connected"
	case connectionReconnecting:
		return "reconnecting"
	default:
		return "connecting"
	}
}

func sessionConnection(snapshot controllerSnapshot, agentID string) connectionPhase {
	if strings.TrimSpace(agentID) == "" {
		agentID = controllerAgentID
	}
	if phase, ok := snapshot.AgentConnections[agentID]; ok {
		return phase
	}
	return snapshot.Connection
}

func anyAgentConnected(snapshot controllerSnapshot) bool {
	for _, phase := range snapshot.AgentConnections {
		if phase == connectionConnected {
			return true
		}
	}
	return snapshot.Connection == connectionConnected
}

func (s *shell) layoutAgentProfilesPanel(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	profiles := snapshot.AgentProfiles
	enabled := !snapshot.AgentUpdating
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPanelTitle(gtx, "ACP agents")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			summary := "One ACP process"
			if len(profiles) != 1 {
				summary = strconv.Itoa(len(profiles)) + " supervised ACP processes"
			}
			return s.layoutLabel(gtx, summary, textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 2)
		}),
	}

	for _, profile := range profiles {
		profile := profile
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutAgentProfileRow(gtx, profile, profile.ID == s.agentEditorOriginalID, enabled)
		}))
	}

	toggleLabel := "Add agent"
	if s.agentEditorVisible {
		toggleLabel = "Hide editor"
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutButton(gtx, &s.agentFormToggleButton, toggleLabel, enabled, func() {
				if s.agentEditorVisible {
					s.agentEditorVisible = false
					s.agentEditorOriginalID = ""
					s.agentEditorKey = ""
					s.clearAgentProfileEditors()
					return
				}
				s.agentEditorVisible = true
				s.agentEditorOriginalID = ""
				s.agentEditorKey = ""
				s.clearAgentProfileEditors()
			})
		})
	}))

	if s.agentEditorVisible {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutAgentProfileEditor(gtx, "Agent ID", &s.agentIDEditor, enabled)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutAgentProfileEditor(gtx, "Display name", &s.agentNameEditor, enabled)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutAgentProfileEditor(gtx, "Command", &s.agentCommandEditor, enabled)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutAgentProfileEditor(gtx, "Arguments JSON", &s.agentArgsEditor, enabled)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutAgentProfileEditor(gtx, "Environment keys JSON", &s.agentEnvEditor, enabled)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &s.agentSaveButton, "Save agent", enabled && strings.TrimSpace(s.agentIDEditor.Text()) != "" && strings.TrimSpace(s.agentCommandEditor.Text()) != "", func() {
					s.onSaveAgentProfile(s.agentEditorOriginalID, s.agentIDEditor.Text(), s.agentNameEditor.Text(), s.agentCommandEditor.Text(), s.agentArgsEditor.Text(), s.agentEnvEditor.Text())
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &s.agentRemoveButton, "Remove agent", enabled && s.agentEditorOriginalID != "" && len(profiles) > 1, func() {
					s.onRemoveAgentProfile(s.agentEditorOriginalID)
				})
			}),
		)
	}

	status := "Changes apply after restarting Desktop. Environment values are read at process start and are never stored."
	statusColor := s.theme.onSurfaceVariant
	if snapshot.AgentError != "" {
		status = "ACP settings unavailable · " + compactInspectorText(snapshot.AgentError, 240)
		statusColor = s.theme.onErrorContainer
	} else if snapshot.AgentConfigOverridden {
		status = "PROTONMAN_ACP_AGENTS_JSON is active. Remove the override and restart to apply saved profiles."
	} else if snapshot.AgentUpdating {
		status = "Saving ACP agent settings…"
	}
	children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return s.layoutLabel(gtx, status, textLabelMedium, font.Normal, statusColor, 4)
	}))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutAgentProfileRow(gtx layout.Context, profile app.ACPAgentProfile, selected, enabled bool) layout.Dimensions {
	button := s.agentProfileButtons[profile.ID]
	rowContext := gtx
	if !enabled {
		rowContext = gtx.Disabled()
	}
	if enabled && button.Clicked(rowContext) {
		s.agentEditorOriginalID = profile.ID
		s.agentEditorKey = ""
		s.agentEditorVisible = true
	}
	gtx.Constraints.Min.Y = gtx.Dp(44)
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
		semantic.DescriptionOp("Edit ACP agent " + profile.DisplayName).Add(gtx.Ops)
		return s.roundedSurface(gtx, 12, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, profile.DisplayName+" · "+profile.ID, textBodyMedium, font.Medium, foreground, 1)
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

func (s *shell) layoutAgentProfileEditor(gtx layout.Context, label string, editor *widget.Editor, enabled bool) layout.Dimensions {
	if enabled {
		for {
			if _, ok := editor.Update(gtx); !ok {
				break
			}
		}
	}
	return s.layoutInspectorEditor(gtx, label, editor, enabled)
}

func (s *shell) clearAgentProfileEditors() {
	s.agentIDEditor.SetText("")
	s.agentNameEditor.SetText("")
	s.agentCommandEditor.SetText("")
	s.agentArgsEditor.SetText("[]")
	s.agentEnvEditor.SetText("[]")
}
