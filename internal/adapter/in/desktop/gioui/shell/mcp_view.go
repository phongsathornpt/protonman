//go:build desktop || desktop_gio

package shell

import (
	"gioui.org/layout"

	settingscomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/settings"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
)

func (s *Shell) syncMCPIntegrationEditors(state desktopstate.State) {
	items := make([]settingscomponent.MCPIntegration, 0, len(state.Integrations))
	for _, item := range state.Integrations {
		items = append(items, settingscomponent.MCPIntegration{
			Name: item.Name, Command: item.Command, Args: item.Args, Env: item.Env,
		})
	}
	s.settingsComponent.SyncMCPIntegrationEditors(s.syncRevision, items)
}

func (s *Shell) layoutMCPIntegrationsPanel(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	items := make([]settingscomponent.MCPIntegration, 0, len(snapshot.State.Integrations))
	for _, item := range snapshot.State.Integrations {
		items = append(items, settingscomponent.MCPIntegration{
			Name: item.Name, Command: item.Command, Args: item.Args, Env: item.Env,
		})
	}
	busy := false
	for _, session := range snapshot.State.Sessions {
		if controller.SessionBusy(session.Status) {
			busy = true
			break
		}
	}
	return s.settingsComponent.LayoutMCP(gtx, settingscomponent.MCPInput{
		Integrations: items, Updating: snapshot.MCPUpdating, Reconnecting: snapshot.MCPReconnecting,
		Connected: anyAgentConnected(snapshot), SessionBusy: busy, Error: snapshot.MCPError,
		Chrome: s.settingsChrome(), OnSave: s.bind.SaveMCPIntegration,
		OnRemove: s.bind.RemoveMCPIntegration, OnReconnect: s.bind.ReconnectMCP,
	})
}
