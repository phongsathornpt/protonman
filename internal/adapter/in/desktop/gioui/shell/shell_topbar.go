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
	"gioui.org/unit"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// The window's top bar: project breadcrumb, connection state,
// and the runtime controls that sit above the conversation pane.
func (s *Shell) layoutTopBar(gtx layout.Context, snapshot controller.Snapshot, windowWidth unit.Dp) layout.Dimensions {
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
						agentName := compactInspectorText(activeAgentDisplayName(snapshot), 16)
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, agentName, textLabelSmall, font.Medium, s.theme.Colors.OnSurfaceVariant, 1)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return uikit.Inset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutConnectionPill(gtx, snapshot)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutIconActionButton(gtx, &s.inspectorToggle, uikit.KindInspector, "Toggle inspector", func() {
									s.inspectorComponent.ToggleForWidth(windowWidth, s.sidebarData.Visible())
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
	connected := snapshot.Connection == controller.ConnectionConnected
	if !connected {
		background = s.theme.Colors.WarningContainer
		foreground = s.theme.Colors.OnWarningContainer
	}
	status := strings.ToLower(snapshot.Status)
	// Soft failures keep the last transcript/runtime state visible but must
	// never paint the pill red while the ACP connection itself is healthy.
	// Only true transport drops (connection/start/disconnected) go red.
	softFailure := strings.Contains(status, "permission") ||
		strings.Contains(status, "prompt failed") ||
		strings.Contains(status, "prompt not sent") ||
		strings.Contains(status, "cancel failed") ||
		strings.Contains(status, "history failed") ||
		strings.Contains(status, "session refresh failed") ||
		strings.Contains(status, "session resume failed") ||
		strings.Contains(status, "new session failed") ||
		strings.Contains(status, "runtime update failed") ||
		strings.Contains(status, "runtime not updated") ||
		strings.Contains(status, "runtime update returned") ||
		strings.Contains(status, "saving preferences failed") ||
		strings.Contains(status, "mcp settings save failed")
	transportFailure := strings.Contains(status, "connection failed") ||
		strings.Contains(status, "start failed") ||
		strings.Contains(status, "disconnected")
	if softFailure {
		background = s.theme.Colors.WarningContainer
		foreground = s.theme.Colors.OnWarningContainer
	} else if transportFailure {
		background = s.theme.Colors.ErrorContainer
		foreground = s.theme.Colors.OnErrorContainer
	} else if !connected {
		if strings.Contains(status, "failed") || (strings.Contains(status, "unavailable") && !strings.Contains(status, "session list unavailable")) {
			background = s.theme.Colors.ErrorContainer
			foreground = s.theme.Colors.OnErrorContainer
		}
	} else if strings.Contains(status, "failed") || (strings.Contains(status, "unavailable") && !strings.Contains(status, "session list unavailable")) {
		// Connected with any other failure text (e.g. future runtime
		// messages): warn, do not misreport as a connection drop.
		background = s.theme.Colors.WarningContainer
		foreground = s.theme.Colors.OnWarningContainer
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
	case strings.Contains(lower, "mcp settings save failed"):
		return "MCP save failed"
	case strings.Contains(lower, "prompt not sent"):
		return "Not sent"
	case strings.Contains(lower, "history failed"):
		return "History failed"
	case strings.Contains(lower, "session refresh failed"):
		return "Refresh failed"
	case strings.Contains(lower, "session resume failed"):
		return "Resume failed"
	case strings.Contains(lower, "new session failed"):
		return "New session failed"
	case strings.Contains(lower, "saving preferences failed"):
		return "Save failed"
	case strings.Contains(lower, "prompt failed"):
		return "Prompt failed"
	case strings.Contains(lower, "runtime update failed"):
		return "Runtime failed"
	case strings.Contains(lower, "runtime not updated"):
		return "Not updated"
	case strings.Contains(lower, "runtime update returned"):
		return "Runtime failed"
	case strings.Contains(lower, "runtime updated"):
		return "Runtime updated"
	case strings.Contains(lower, "updating runtime"):
		return "Updating…"
	case strings.Contains(lower, "permission"):
		return "Permission required"
	case strings.Contains(lower, "start failed"):
		return "Start failed"
	case strings.Contains(lower, "unavailable"):
		return "Unavailable"
	case strings.Contains(lower, "cancel failed"):
		return "Cancel failed"
	case strings.Contains(lower, "connection failed"):
		return "Connection failed"
	case strings.Contains(lower, "disconnected"):
		return "Disconnected"
	case strings.Contains(lower, "reconnecting"):
		return "Reconnecting"
	case strings.Contains(lower, "connecting"):
		return "Connecting"
	case strings.Contains(lower, "connected"):
		return "Connected"
	case strings.Contains(lower, "failed") || strings.Contains(lower, "error") || strings.Contains(lower, "invalid"):
		return "Action failed"
	}
	return compactInspectorText(status, 26)
}
