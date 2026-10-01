//go:build desktop || desktop_gio

package shell

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"

	runtimecomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/runtime"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
)

func (s *Shell) openModelPopover() {
	if !s.runtimeComponent.Widgets().ModelPopoverVisible && !s.runtimeComponent.Widgets().ReasoningPopoverVisible && !s.runtimeComponent.Widgets().PermissionModePopoverVisible && !s.conversationUI.Timeline().Position.BeforeEnd {
		s.tailFollowBeforeOverlay = true
	}
	s.runtimeComponent.Widgets().ModelPopoverVisible = true
	s.runtimeComponent.Widgets().ReasoningPopoverVisible = false
	s.runtimeComponent.Widgets().PermissionModePopoverVisible = false
	s.runtimeComponent.Widgets().ModelSearchFocusPending = true
	s.mentionActive = false
	s.runtimeComponent.Widgets().PopoverActiveProviderTab = ""
	if s.bind.RefreshRuntime != nil {
		s.bind.RefreshRuntime()
	}
	if s.bind.RefreshProviders != nil {
		s.bind.RefreshProviders()
	}
}

func (s *Shell) openReasoningPopover() {
	wasAtTail := !s.conversationUI.Timeline().Position.BeforeEnd
	s.closePopovers()
	if wasAtTail {
		s.tailFollowBeforeOverlay = true
	}
	s.runtimeComponent.Widgets().ReasoningPopoverVisible = true
	s.mentionActive = false
}

func (s *Shell) openPermissionModePopover() {
	wasAtTail := !s.conversationUI.Timeline().Position.BeforeEnd
	s.closePopovers()
	if wasAtTail {
		s.tailFollowBeforeOverlay = true
	}
	s.runtimeComponent.Widgets().PermissionModePopoverVisible = true
	s.mentionActive = false
}

func (s *Shell) closePopovers() {
	s.runtimeComponent.ClosePopovers()
	s.runtimeComponent.Widgets().ModelSearchFocusPending = false
	if s.tailFollowBeforeOverlay && !s.mentionActive {
		s.conversationUI.Timeline().ScrollToEnd = true
		s.conversationUI.Timeline().Position = layout.Position{}
		s.tailFollowBeforeOverlay = false
	}
}

func (s *Shell) popoverPermissionModeButton(mode string) *widget.Clickable {
	return s.runtimeComponent.PopoverPermissionModeButton(mode)
}

func (s *Shell) agentModelButton(name string) *widget.Clickable {
	return s.runtimeComponent.AgentModelButton(name)
}

func (s *Shell) modelPresetButton(name string) *widget.Clickable {
	return s.runtimeComponent.ModelPresetButton(name)
}

func (s *Shell) popoverReasoningButton(level string) *widget.Clickable {
	return s.runtimeComponent.PopoverReasoningButton(level)
}

func (s *Shell) popoverProviderTabButton(id string) *widget.Clickable {
	return s.runtimeComponent.PopoverProviderTabButton(id)
}

func (s *Shell) addRecentModel(provider, model, name string) {
	s.runtimeComponent.AddRecentModel(provider, model, name)
	s.recentModels = s.runtimeComponent.RecentModels()
}

func (s *Shell) runtimeChrome() runtimecomponent.Chrome {
	return s.baseChrome()
}

func (s *Shell) layoutModelPopover(gtx layout.Context, session desktopstate.SessionState, snapshot controller.Snapshot, enabled bool) layout.Dimensions {
	return s.runtimeComponent.LayoutModelPopover(gtx, runtimecomponent.ModelPopoverInput{
		Session:              session,
		ActiveAgentID:        snapshot.ActiveAgentID,
		AgentProfiles:        snapshot.AgentProfiles,
		AgentDefaultModels:   snapshot.AgentDefaultModels,
		ActiveProvider:       snapshot.ActiveProvider,
		Providers:            snapshot.Providers,
		ProviderModels:       snapshot.ProviderModels,
		AgentAvailableModels: snapshot.AgentAvailableModels,
		Enabled:              enabled,
		Chrome:               s.runtimeChrome(),
		OnSetModel: func(provider, model string) {
			if s.bind.SetRuntimeModel != nil {
				s.bind.SetRuntimeModel(provider, model)
			}
		},
		OnFetchProviderModels: func(id, name, baseURL, apiKey string, cb func([]string, error)) {
			if s.bind.FetchProviderModels != nil {
				s.bind.FetchProviderModels(id, name, baseURL, apiKey, cb)
			}
		},
		OnRefreshRuntime: s.bind.RefreshRuntime,
		OnManageProviders: func() {
			s.settingsComponent.SetActiveTab(1)
			s.closePopovers()
			s.openSettingsModal()
		},
		OnClose: func() {
			s.closePopovers()
			gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
		},
	})
}

func (s *Shell) layoutReasoningPopover(gtx layout.Context, session desktopstate.SessionState, enabled bool) layout.Dimensions {
	return s.runtimeComponent.LayoutReasoningPopover(gtx, runtimecomponent.ReasoningPopoverInput{
		Session: session,
		Enabled: enabled,
		Chrome:  s.runtimeChrome(),
		OnSetReasoning: func(level string) {
			if s.bind.SetRuntimeReasoning != nil {
				s.bind.SetRuntimeReasoning(level)
			}
		},
		OnClose: func() {
			s.closePopovers()
			gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
		},
	})
}

func (s *Shell) layoutPermissionModePopover(gtx layout.Context, session desktopstate.SessionState, enabled bool) layout.Dimensions {
	return s.runtimeComponent.LayoutPermissionModePopover(gtx, runtimecomponent.PermissionModePopoverInput{
		Session: session,
		Enabled: enabled,
		Chrome:  s.runtimeChrome(),
		OnSetPermissionMode: func(mode string) {
			if s.bind.SetRuntimePermissionMode != nil {
				s.bind.SetRuntimePermissionMode(mode)
			}
		},
		OnClose: func() {
			s.closePopovers()
			gtx.Execute(key.FocusCmd{Tag: s.conversationUI.Editor()})
		},
	})
}
