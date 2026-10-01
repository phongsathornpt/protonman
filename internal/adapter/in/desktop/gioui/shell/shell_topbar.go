//go:build desktop || desktop_gio

package shell

import (
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	inspectorcomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/inspector"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// The window's top bar: project breadcrumb, agent selector, connection state,
// and the runtime controls that sit above the conversation pane.
func (s *Shell) layoutTopBar(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(60)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, s.theme.Colors.Surface, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}.Op())
			return uikit.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return uikit.Inset{Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							tip := "Show sidebar"
							if s.sidebarData.Visible() {
								tip = "Hide sidebar"
							}
							return s.layoutIconActionButton(gtx, s.sidebarData.ToggleButton(), uikit.KindSidebar, tip, func() {
								s.sidebarData.ToggleVisible()
							})
						})
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						session, ok := selectedSession(snapshot.State)
						title, workspace := "New conversation", "Choose a workspace to get started"
						if ok {
							title = strings.TrimSpace(session.Title)
							if title == "" {
								title = "Untitled conversation"
							}
							workspace = strings.TrimSpace(session.WorkspaceName)
							if workspace == "" {
								workspace = strings.TrimSpace(session.Workspace)
							}
							if project := projectDisplayName(snapshot.State, session.ProjectID); project != "" {
								workspace = project
							}
							if workspace == "" {
								workspace = "Workspace"
							}
						}
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, compactInspectorText(title, 68), textTitleMedium, font.SemiBold, s.theme.Colors.OnSurface, 1)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, compactInspectorText(workspace, 56), textLabelSmall, font.Normal, s.theme.Colors.OnSurfaceVariant, 1)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						wideInspector := gtx.Constraints.Max.X >= gtx.Dp(inspectorcomponent.WideBreakpoint)
						agentName := compactInspectorText(activeAgentDisplayName(snapshot), 16)
						chevronSuffix := " ▾"
						if s.settingsComponent.AgentSelectorVisible() {
							chevronSuffix = " ▴"
						}
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutAgentCapsuleButton(gtx, &s.settingsComponent.AgentWidgets().SelectorButton, agentName+chevronSuffix, func() {
										s.settingsComponent.ToggleAgentSelector()
									})
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutConnectionPill(gtx, snapshot)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutIconActionButton(gtx, &s.inspectorToggle, uikit.KindInspector, "Toggle inspector", func() {
									s.inspectorComponent.Toggle(wideInspector)
								})
							}),
						)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutHorizontalDivider(gtx)
		}),
	)
}

func projectDisplayName(state desktopstate.State, projectID string) string {
	for _, project := range state.Projects {
		if project.ID == projectID {
			return strings.TrimSpace(project.Name)
		}
	}
	return ""
}

func (s *Shell) layoutConnectionPill(gtx layout.Context, snapshot controller.Snapshot) layout.Dimensions {
	background := s.theme.Colors.SuccessContainer
	foreground := s.theme.Colors.OnSuccessContainer
	if snapshot.Connection != controller.ConnectionConnected {
		background = s.theme.Colors.WarningContainer
		foreground = s.theme.Colors.OnWarningContainer
	}
	status := strings.ToLower(snapshot.Status)
	if strings.Contains(status, "permission") {
		background = s.theme.Colors.WarningContainer
		foreground = s.theme.Colors.OnWarningContainer
	}
	if strings.Contains(status, "failed") || (strings.Contains(status, "unavailable") && !strings.Contains(status, "session list unavailable")) || strings.Contains(status, "disconnected") {
		background = s.theme.Colors.ErrorContainer
		foreground = s.theme.Colors.OnErrorContainer
	}
	gtx.Constraints.Min.Y = gtx.Dp(28)
	return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
		return uikit.Inset{Top: 4, Bottom: 4, Left: 10, Right: 10}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				semantic.DescriptionOp(strings.TrimSpace(snapshot.Status)).Add(gtx.Ops)
				return s.layoutLabel(gtx, connectionStatusLabel(snapshot.Status), textLabelMedium, font.SemiBold, foreground, 1)
			},
		)
	})
}

func connectionStatusLabel(status string) string {
	status = strings.TrimSpace(status)
	const projectSelectedPrefix = "Project selected · "
	if strings.HasPrefix(status, projectSelectedPrefix) {
		return compactInspectorText("Project · "+strings.TrimPrefix(status, projectSelectedPrefix), 30)
	}
	lower := strings.ToLower(status)
	switch {
	case strings.Contains(lower, "session list unavailable"):
		return "Connected"
	case strings.Contains(lower, "mcp settings unavailable"):
		return "MCP unavailable"
	case strings.Contains(lower, "history failed"):
		return "History failed"
	case strings.Contains(lower, "prompt failed"):
		return "Prompt failed"
	case strings.Contains(lower, "permission"):
		return "Permission required"
	case strings.Contains(lower, "unavailable"):
		return "Unavailable"
	case strings.Contains(lower, "failed"):
		return "Connection failed"
	case strings.Contains(lower, "disconnected"):
		return "Disconnected"
	case strings.Contains(lower, "reconnecting"):
		return "Reconnecting"
	case strings.Contains(lower, "connecting"):
		return "Connecting"
	case strings.Contains(lower, "connected"):
		return "Connected"
	}
	return compactInspectorText(status, 26)
}

func desktopIconKind(glyph string) (uikit.IconKind, bool) {
	switch glyph {
	case "◧":
		return uikit.KindSidebar, true
	case "◨":
		return uikit.KindInspector, true
	case "+":
		return uikit.KindAdd, true
	case "✕":
		return uikit.KindClose, true
	default:
		return 0, false
	}
}
