//go:build desktop || desktop_gio

package shell

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	settingscomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/settings"
	"github.com/phongsathornpt/protonman/internal/app"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
)

func activeAgentDisplayName(snapshot controller.Snapshot) string {
	return app.AgentDisplayName(snapshot.AgentProfiles, snapshot.ActiveAgentID)
}

func (s *Shell) syncAgentProfileEditors(snapshot controller.Snapshot) {
	if s.syncRevision != 0 && s.settingsComponent.AgentWidgets().SyncRevisionSet && s.settingsComponent.AgentWidgets().SyncRevision == s.syncRevision {
		return
	}
	live := s.settingsComponent.AgentWidgets().ProfileLive
	if live == nil {
		live = make(map[string]struct{}, len(snapshot.AgentProfiles))
		s.settingsComponent.AgentWidgets().ProfileLive = live
	}
	clear(live)
	for _, profile := range snapshot.AgentProfiles {
		live[profile.ID] = struct{}{}
		if s.settingsComponent.AgentWidgets().ProfileButtons[profile.ID] == nil {
			s.settingsComponent.AgentWidgets().ProfileButtons[profile.ID] = new(widget.Clickable)
		}
	}
	for agentID := range s.settingsComponent.AgentWidgets().ProfileButtons {
		if _, ok := live[agentID]; !ok {
			delete(s.settingsComponent.AgentWidgets().ProfileButtons, agentID)
		}
	}
	agentIDs := make([]string, 0, len(snapshot.AgentProfiles))
	for _, profile := range snapshot.AgentProfiles {
		agentIDs = append(agentIDs, profile.ID)
	}
	s.settingsComponent.SyncAgentChoices(agentIDs)

	if s.settingsComponent.AgentWidgets().EditorOriginalID != "" {
		if _, ok := live[s.settingsComponent.AgentWidgets().EditorOriginalID]; !ok {
			s.settingsComponent.AgentWidgets().EditorOriginalID = ""
			s.settingsComponent.AgentWidgets().EditorKey = ""
			s.settingsComponent.AgentWidgets().EditorVisible = false
			s.clearAgentProfileEditors()
		}
	}
	if !s.settingsComponent.AgentWidgets().EditorVisible || s.settingsComponent.AgentWidgets().EditorOriginalID == "" || s.settingsComponent.AgentWidgets().EditorOriginalID == s.settingsComponent.AgentWidgets().EditorKey {
		if s.syncRevision != 0 {
			s.settingsComponent.AgentWidgets().SyncRevision = s.syncRevision
			s.settingsComponent.AgentWidgets().SyncRevisionSet = true
		}
		return
	}
	for _, profile := range snapshot.AgentProfiles {
		if profile.ID != s.settingsComponent.AgentWidgets().EditorOriginalID {
			continue
		}
		s.settingsComponent.AgentWidgets().EditorKey = profile.ID
		s.settingsComponent.AgentWidgets().IDEditor.SetText(profile.ID)
		s.settingsComponent.AgentWidgets().NameEditor.SetText(profile.DisplayName)
		s.settingsComponent.AgentWidgets().CommandEditor.SetText(profile.Command)
		s.settingsComponent.AgentWidgets().ArgsEditor.SetText(mcpJSONList(profile.Args))
		s.settingsComponent.AgentWidgets().EnvEditor.SetText(mcpJSONList(profile.Env))
		if s.syncRevision != 0 {
			s.settingsComponent.AgentWidgets().SyncRevision = s.syncRevision
			s.settingsComponent.AgentWidgets().SyncRevisionSet = true
		}
		return
	}
	if s.syncRevision != 0 {
		s.settingsComponent.AgentWidgets().SyncRevision = s.syncRevision
		s.settingsComponent.AgentWidgets().SyncRevisionSet = true
	}
}

func (s *Shell) layoutAgentSelectorBar(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	profiles := make([]settingscomponent.AgentChoice, 0, len(snapshot.AgentProfiles))
	for _, profile := range snapshot.AgentProfiles {
		profiles = append(profiles, settingscomponent.AgentChoice{
			ID: profile.ID, DisplayName: profile.DisplayName,
			Connection: string(snapshot.AgentConnections[profile.ID]),
		})
	}
	return s.settingsComponent.LayoutAgentSelector(gtx, settingscomponent.AgentSelectorInput{
		Profiles: profiles, ActiveID: snapshot.ActiveAgentID, Chrome: s.settingsChrome(),
		OnSelect: s.bind.SelectAgent,
	})
}

func sessionConnection(snapshot controller.Snapshot, agentID string) controller.ConnectionPhase {
	if strings.TrimSpace(agentID) == "" {
		agentID = controller.ProtonmanAgentID
	}
	if phase, ok := snapshot.AgentConnections[agentID]; ok {
		return phase
	}
	return snapshot.Connection
}

func anyAgentConnected(snapshot controller.Snapshot) bool {
	for _, phase := range snapshot.AgentConnections {
		if phase == controller.ConnectionConnected {
			return true
		}
	}
	return snapshot.Connection == controller.ConnectionConnected
}

func (s *Shell) layoutStatusDot(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	size := gtx.Dp(8)
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	paint.FillShape(gtx.Ops, c, clip.Ellipse{Max: image.Pt(size, size)}.Op(gtx.Ops))
	return layout.Dimensions{Size: image.Pt(size, size)}
}

func (s *Shell) agentsInput(snapshot controller.Snapshot, enabled bool) settingscomponent.AgentsInput {
	connections := make(map[string]string, len(snapshot.AgentConnections))
	for id, phase := range snapshot.AgentConnections {
		connections[id] = string(phase)
	}
	return settingscomponent.AgentsInput{
		Profiles:              snapshot.AgentProfiles,
		ActiveAgentID:         snapshot.ActiveAgentID,
		AgentConnections:      connections,
		AgentStatuses:         snapshot.AgentStatuses,
		AgentError:            snapshot.AgentError,
		AgentConfigOverridden: snapshot.AgentConfigOverridden,
		Updating:              snapshot.AgentUpdating,
		Chrome:                s.settingsChrome(),
		OnSave: func(origID, newID, name, cmd, args, env string) {
			s.bind.SaveAgentProfile(origID, newID, name, cmd, args, env)
		},
		OnRemove: func(id string) {
			s.bind.RemoveAgentProfile(id)
		},
		OnScanDevice: func() {
			s.bind.ScanDeviceAgents()
		},
	}
}

func (s *Shell) clearAgentProfileEditors() {
	s.settingsComponent.ClearAgentProfileEditors()
}

func (s *Shell) layoutAgentProfilesPanel(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	return s.settingsComponent.LayoutAgents(gtx, s.agentsInput(snapshot, !snapshot.AgentUpdating))
}
