//go:build desktop || desktop_gio

package shell

import (
	"log"

	"gioui.org/io/key"
	"gioui.org/layout"

	settingscomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/settings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
)

func (s *Shell) openSettingsModal() {
	s.settingsComponent.Show()
	s.closePopovers()
	s.mentionActive = false
	log.Printf("[UI] settings modal opened, activeTab=%d", s.settingsComponent.ActiveTab())
}

func (s *Shell) closeSettingsModal() {
	s.settingsComponent.Close()
	log.Printf("[UI] settings modal closed")
}

func (s *Shell) layoutSettingsModal(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	return s.settingsComponent.Layout(gtx, settingscomponent.ViewInput{
		Chrome: s.settingsChrome(),
		Content: func(gtx layout.Context, tab int) layout.Dimensions {
			return s.layoutSettingsContent(gtx, snapshot, tab)
		},
		OnClose: func() { gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()}) },
	})
}

func (s *Shell) settingsChrome() settingscomponent.Chrome {
	return s.baseChrome()
}

func (s *Shell) layoutSettingsContent(gtx layout.Context, snapshot controller.Snapshot, tab int) layout.Dimensions {
	if gtx.Constraints.Max.X > 0 {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
	}
	switch tab {
	case settingscomponent.ProvidersTab:
		return s.layoutProvidersPanel(gtx, snapshot)
	case settingscomponent.MCPTab:
		return s.layoutMCPIntegrationsPanel(gtx, snapshot)
	case settingscomponent.AgentsTab:
		return s.layoutAgentProfilesPanel(gtx, snapshot)
	default:
		return s.layoutSettingsGeneralTab(gtx, snapshot)
	}
}

func (s *Shell) layoutSettingsGeneralTab(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	return s.settingsComponent.LayoutGeneral(gtx, settingscomponent.GeneralInput{
		Theme: snapshot.Theme, SystemDark: uikit.IsSystemDarkMode(), Chrome: s.settingsChrome(), OnSetTheme: s.bind.SetTheme,
	})
}
