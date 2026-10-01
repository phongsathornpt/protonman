//go:build desktop || desktop_gio

package shell

import (
	"image"
	"os/exec"
	"runtime"

	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	sidebarcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/sidebar"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
)

type sidebarRow = sidebarcomponent.Row

type sidebarRowsCache struct {
	cache      sidebarcomponent.RowsCache
	model      sidebarcomponent.Model
	filterMode string
}

func (cache sidebarRowsCache) matches(state desktopstate.State, profiles []app.ACPAgentProfile) bool {
	model := cache.model
	model.State = state
	model.AgentProfiles = profiles
	return cache.cache.Matches(model)
}

func (cache sidebarRowsCache) matchesWithOptions(state desktopstate.State, profiles []app.ACPAgentProfile, filterMode string, pinnedSessions []string, customTitles map[string]string) bool {
	model := sidebarcomponent.Model{
		State: state, AgentProfiles: profiles, FilterMode: filterMode,
		Pinned: pinnedSessions, CustomTitles: customTitles,
	}
	return cache.cache.Matches(model)
}

const (
	sidebarProjectRow      = sidebarcomponent.ProjectRow
	sidebarSessionRow      = sidebarcomponent.SessionRow
	sidebarPinnedHeaderRow = sidebarcomponent.PinnedHeaderRow
)

func (s *Shell) layoutSidebar(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	return s.sidebarData.Layout(gtx, s.sidebarViewInput(snapshot))
}

func (s *Shell) sidebarViewInput(snapshot controller.Snapshot) sidebarcomponent.ViewInput {
	return sidebarcomponent.ViewInput{
		Snapshot: sidebarcomponent.Snapshot{
			State: snapshot.State, AgentProfiles: snapshot.AgentProfiles,
			PinnedSessions: snapshot.PinnedSessions, CustomTitles: snapshot.CustomTitles,
			FilterMode: snapshot.FilterMode, Revision: snapshot.Revision,
			Connection: string(snapshot.Connection), Status: snapshot.Status,
			CreatingSession: snapshot.CreatingSession, SettingsModalOpen: s.settingsComponent.IsOpen(),
		},
		Actions: sidebarcomponent.Actions{
			SelectSession: s.bind.SelectSession,
			SelectProject: s.bind.SelectProject,
			NewSession:    s.bind.NewSession,
			DeleteSession: s.bind.DeleteSession,
			RenameSession: s.bind.RenameSession,
			TogglePin:     s.bind.TogglePinSession,
			SetFilterMode: s.bind.SetFilterMode,
			OpenSettings:  s.openSettingsModal,
			OpenCommunity: func() { openBrowserURL("https://github.com/phongsathornpt/protonman") },
		},
		Chrome: s.sidebarChrome(),
	}
}

func (s *Shell) sidebarChrome() sidebarcomponent.Chrome {
	return sidebarcomponent.Chrome{
		Chrome:     s.baseChrome(),
		TaskStatus: s.layoutSidebarTaskStatus,
	}
}

func (s *Shell) sidebarRows(snapshot controller.Snapshot) []sidebarRow {
	return s.sidebarData.Rows(sidebarcomponent.Model{
		State: snapshot.State, AgentProfiles: snapshot.AgentProfiles,
		Pinned: snapshot.PinnedSessions, CustomTitles: snapshot.CustomTitles,
		FilterMode: snapshot.FilterMode, Revision: snapshot.Revision,
	})
}

func (s *Shell) sidebarDisplayRows(rows []sidebarRow, filterMode string, sourceRevisions ...uint64) []sidebarRow {
	sourceRevision := uint64(0)
	if len(sourceRevisions) > 0 {
		sourceRevision = sourceRevisions[0]
	}
	return s.sidebarData.DisplayRows(rows, s.sidebarData.SearchQuery(), filterMode, sourceRevision)
}

func buildSidebarRows(state desktopstate.State, profiles []app.ACPAgentProfile) ([]sidebarRow, sidebarRowsCache) {
	return buildSidebarRowsWithOptions(state, profiles, nil, nil)
}

func buildSidebarRowsWithOptions(state desktopstate.State, profiles []app.ACPAgentProfile, pinnedSessions []string, customTitles map[string]string) ([]sidebarRow, sidebarRowsCache) {
	model := sidebarcomponent.Model{State: state, AgentProfiles: profiles, Pinned: pinnedSessions, CustomTitles: customTitles}
	cache := sidebarcomponent.BuildRows(model)
	return cache.Rows(), sidebarRowsCache{cache: cache, model: model, filterMode: model.FilterMode}
}

func sidebarSessionWidgetKey(sessionID string, agentIDs ...string) string {
	return sidebarcomponent.SessionWidgetKey(sessionID, agentIDs...)
}

func (s *Shell) sessionMenuButton(sessionID string, agentIDs ...string) *widget.Clickable {
	return s.sidebarData.MenuButton(sessionID, agentIDs...)
}

func (s *Shell) sessionPinButton(sessionID string, agentIDs ...string) *widget.Clickable {
	return s.sidebarData.PinButton(sessionID, agentIDs...)
}

func (s *Shell) sessionQuickRenameButton(sessionID string, agentIDs ...string) *widget.Clickable {
	return s.sidebarData.RenameButton(sessionID, agentIDs...)
}

func (s *Shell) sessionQuickDeleteButton(sessionID string, agentIDs ...string) *widget.Clickable {
	return s.sidebarData.DeleteButton(sessionID, agentIDs...)
}

func (s *Shell) syncSessionButtons(state desktopstate.State, revision uint64) {
	s.sidebarData.SyncSessionButtons(state, revision)
}

func (s *Shell) layoutSidebarTaskStatus(gtx layout.Context, label string, status desktopstate.TaskStatus) layout.Dimensions {
	background, foreground := s.taskStatusColors(status)
	return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, label, textLabelSmall, font.SemiBold, foreground, 1)
		})
	})
}

// layoutMiniIconButton remains a shell primitive used by non-sidebar views.
func (s *Shell) layoutMiniIconButton(gtx layout.Context, button *widget.Clickable, label string, normalColor color.NRGBA) layout.Dimensions {
	background := color.NRGBA{}
	foreground := normalColor
	if button.Hovered() {
		background = s.theme.Colors.SurfaceContainerHighest
		foreground = s.theme.Colors.OnSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Pt(gtx.Dp(22), gtx.Dp(22))
		gtx.Constraints.Max = image.Pt(gtx.Dp(22), gtx.Dp(22))
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if label == "✕" || label == "×" {
					return uikit.LayoutActionIcon(gtx, uikit.KindClose, 12, foreground)
				}
				return s.layoutLabel(gtx, label, textLabelSmall, font.Bold, foreground, 1)
			})
		})
	})
	return dims
}

func openBrowserURL(targetURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}
