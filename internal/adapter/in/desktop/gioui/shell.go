//go:build desktop || desktop_gio

package gioui

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/markdown"

	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type sidebarRowKind uint8

const (
	sidebarProjectRow sidebarRowKind = iota
	sidebarSessionRow
)

type sidebarRow struct {
	Kind           sidebarRowKind
	ProjectID      string
	SessionID      string
	AgentID        string
	Title          string
	Subtitle       string
	Status         string
	SessionCount   int
	LastActivityAt time.Time
}

type sidebarProjectCache struct {
	id   string
	name string
}

type sidebarSessionCache struct {
	id             string
	projectID      string
	agentID        string
	title          string
	subtitle       string
	status         string
	lastActivityAt time.Time
}

type sidebarRowsCache struct {
	valid    bool
	rows     []sidebarRow
	projects []sidebarProjectCache
	sessions []sidebarSessionCache
}

type shell struct {
	theme *theme

	sidebarList                  layout.List
	sessionButtons               map[string]*widget.Clickable
	projectButtons               map[string]*widget.Clickable
	sessionButtonLive            map[string]struct{}
	projectButtonLive            map[string]struct{}
	buttonRevision               uint64
	buttonRevisionSet            bool
	sidebarRowsCache             sidebarRowsCache
	sidebarNewSessionButton      widget.Clickable
	emptyNewSessionButton        widget.Clickable
	conversationList             layout.List
	conversationMarkdown         *markdown.Renderer
	conversationCache            map[conversationCacheKey]conversationMarkdownCache
	conversationCacheBytes       int
	conversationExpanded         map[conversationCacheKey]bool
	conversationPage             map[conversationCacheKey]int
	conversationExpandButtons    map[conversationCacheKey]*conversationDisclosureButtons
	conversationThinkingExpanded map[conversationCacheKey]bool
	conversationThinkingButtons  map[conversationCacheKey]*widget.Clickable
	inspectorList                layout.List
	inspectorVisible             bool
	inspectorOverride            bool
	inspectorToggle              widget.Clickable
	composer                     widget.Editor
	composerDrafts               map[string]string
	composerDraftOrder           []string
	sendButton                   widget.Clickable
	stopButton                   widget.Clickable
	permissionButtons            map[string]map[string]*widget.Clickable
	permissionButtonLive         map[string]struct{}
	permissionButtonRevision     uint64
	permissionButtonRevisionSet  bool
	runtimeProviderEditor        widget.Editor
	runtimeModelEditor           widget.Editor
	runtimeApplyButton           widget.Clickable
	reasoningButtons             map[string]*widget.Clickable
	lowConcurrencyButtons        map[string]*widget.Clickable
	runtimeEditorKey             string
	mcpNameEditor                widget.Editor
	mcpCommandEditor             widget.Editor
	mcpArgsEditor                widget.Editor
	mcpEnvEditor                 widget.Editor
	mcpSelectedName              string
	mcpEditorKey                 string
	mcpFormVisible               bool
	mcpIntegrationButtons        map[string]*widget.Clickable
	mcpIntegrationLive           map[string]struct{}
	mcpSyncRevision              uint64
	mcpSyncRevisionSet           bool
	mcpFormToggleButton          widget.Clickable
	mcpSaveButton                widget.Clickable
	mcpRemoveButton              widget.Clickable
	mcpReconnectButton           widget.Clickable
	agentSelectorButton          widget.Clickable
	agentSelectorList            layout.List
	agentSelectorVisible         bool
	agentChoiceButtons           map[string]*widget.Clickable
	agentIDEditor                widget.Editor
	agentNameEditor              widget.Editor
	agentCommandEditor           widget.Editor
	agentArgsEditor              widget.Editor
	agentEnvEditor               widget.Editor
	agentProfileButtons          map[string]*widget.Clickable
	agentProfileLive             map[string]struct{}
	agentSyncRevision            uint64
	agentSyncRevisionSet         bool
	agentEditorVisible           bool
	agentEditorOriginalID        string
	agentEditorKey               string
	agentFormToggleButton        widget.Clickable
	agentSaveButton              widget.Clickable
	agentRemoveButton            widget.Clickable
	activeSessionID              string
	syncRevision                 uint64
	onSelectSession              func(string)
	onSelectProject              func(string)
	onNewSession                 func()
	onSendPrompt                 func(string)
	onCancelPrompt               func()
	onResolvePermission          func(string, string)
	onSetRuntimeModel            func(string, string)
	onSetRuntimeReasoning        func(string)
	onSetRuntimeLow              func(string)
	onSaveMCPIntegration         func(string, string, string, string)
	onRemoveMCPIntegration       func(string)
	onReconnectMCP               func()
	onSelectAgent                func(string)
	onSaveAgentProfile           func(string, string, string, string, string, string)
	onRemoveAgentProfile         func(string)
}

func newShell(theme *theme) *shell {
	markdownRenderer := markdown.NewRenderer()
	markdownRenderer.Config.DefaultFont = theme.textFont(font.Normal)
	markdownRenderer.Config.DefaultColor = theme.onSurface
	markdownRenderer.Config.InteractiveColor = theme.primary
	return &shell{
		theme:                        theme,
		sessionButtons:               make(map[string]*widget.Clickable),
		projectButtons:               make(map[string]*widget.Clickable),
		sessionButtonLive:            make(map[string]struct{}),
		projectButtonLive:            make(map[string]struct{}),
		sidebarList:                  layout.List{Axis: layout.Vertical},
		conversationList:             layout.List{Axis: layout.Vertical, ScrollToEnd: true},
		inspectorList:                layout.List{Axis: layout.Vertical},
		composer:                     widget.Editor{Submit: true, MaxLen: 1 << 20},
		composerDrafts:               make(map[string]string),
		runtimeProviderEditor:        widget.Editor{SingleLine: true, MaxLen: 512},
		runtimeModelEditor:           widget.Editor{SingleLine: true, MaxLen: 512},
		mcpNameEditor:                widget.Editor{SingleLine: true, MaxLen: 256},
		mcpCommandEditor:             widget.Editor{SingleLine: true, MaxLen: 1024},
		mcpArgsEditor:                widget.Editor{SingleLine: true, MaxLen: 4096},
		mcpEnvEditor:                 widget.Editor{SingleLine: true, MaxLen: 4096},
		agentSelectorList:            layout.List{Axis: layout.Horizontal},
		agentChoiceButtons:           make(map[string]*widget.Clickable),
		agentIDEditor:                widget.Editor{SingleLine: true, MaxLen: 128},
		agentNameEditor:              widget.Editor{SingleLine: true, MaxLen: 256},
		agentCommandEditor:           widget.Editor{SingleLine: true, MaxLen: 1024},
		agentArgsEditor:              widget.Editor{SingleLine: true, MaxLen: 4096},
		agentEnvEditor:               widget.Editor{SingleLine: true, MaxLen: 4096},
		agentProfileButtons:          make(map[string]*widget.Clickable),
		agentProfileLive:             make(map[string]struct{}),
		conversationMarkdown:         markdownRenderer,
		conversationCache:            make(map[conversationCacheKey]conversationMarkdownCache),
		conversationExpanded:         make(map[conversationCacheKey]bool),
		conversationPage:             make(map[conversationCacheKey]int),
		conversationExpandButtons:    make(map[conversationCacheKey]*conversationDisclosureButtons),
		conversationThinkingExpanded: make(map[conversationCacheKey]bool),
		conversationThinkingButtons:  make(map[conversationCacheKey]*widget.Clickable),
		permissionButtons:            make(map[string]map[string]*widget.Clickable),
		permissionButtonLive:         make(map[string]struct{}),
		reasoningButtons:             make(map[string]*widget.Clickable),
		lowConcurrencyButtons:        make(map[string]*widget.Clickable),
		mcpIntegrationButtons:        make(map[string]*widget.Clickable),
		mcpIntegrationLive:           make(map[string]struct{}),
		onSelectSession:              func(string) {},
		onNewSession:                 func() {},
		onSendPrompt:                 func(string) {},
		onCancelPrompt:               func() {},
		onResolvePermission:          func(string, string) {},
		onSetRuntimeModel:            func(string, string) {},
		onSetRuntimeReasoning:        func(string) {},
		onSetRuntimeLow:              func(string) {},
		onSaveMCPIntegration:         func(string, string, string, string) {},
		onRemoveMCPIntegration:       func(string) {},
		onReconnectMCP:               func() {},
		onSelectProject:              func(string) {},
		onSelectAgent:                func(string) {},
		onSaveAgentProfile:           func(string, string, string, string, string, string) {},
		onRemoveAgentProfile:         func(string) {},
	}
}

func (s *shell) layout(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	s.syncRevision = snapshot.Revision
	s.syncSessionButtons(snapshot.State, snapshot.Revision)
	s.syncConversation(snapshot.State)
	s.syncRuntimeEditors(snapshot.State)
	s.syncMCPIntegrationEditors(snapshot.State)
	s.syncAgentProfileEditors(snapshot)
	paint.Fill(gtx.Ops, s.theme.surface)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 8, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutTopBar(gtx, snapshot)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !s.agentSelectorVisible {
				return layout.Dimensions{}
			}
			return desktopInset{Top: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutAgentSelectorBar(gtx, snapshot)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 8, Bottom: 8, Left: 8, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutSidebar(gtx, snapshot)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 8, Bottom: 8, Left: 6, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutMain(gtx, snapshot)
					})
				}),
			)
		}),
	)
}

func (s *shell) layoutTopBar(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(64)
	return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainer, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return desktopInset{Top: 10, Bottom: 10, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, "Protonman", textHeadlineSmall, font.Bold, s.theme.onSurface, 1)
								}),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Autonomous coding workbench", textLabelMedium, font.Normal, s.theme.onSurfaceVariant, 1)
						}),
					)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Spacer{}.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := "Agent: " + compactInspectorText(activeAgentDisplayName(snapshot), 28)
					if s.agentSelectorVisible {
						label = "Agents: " + compactInspectorText(activeAgentDisplayName(snapshot), 28)
					}
					return desktopUniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutButton(gtx, &s.agentSelectorButton, label, true, func() {
							s.agentSelectorVisible = !s.agentSelectorVisible
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutConnectionPill(gtx, snapshot)
				}),
			)
		})
	})
}

func (s *shell) layoutConnectionPill(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	background := s.theme.successContainer
	foreground := s.theme.onSuccessContainer
	if snapshot.Connection != connectionConnected {
		background = s.theme.warningContainer
		foreground = s.theme.onWarningContainer
	}
	status := strings.ToLower(snapshot.Status)
	if strings.Contains(status, "permission") {
		background = s.theme.warningContainer
		foreground = s.theme.onWarningContainer
	}
	if strings.Contains(status, "failed") || strings.Contains(status, "unavailable") || strings.Contains(status, "disconnected") {
		background = s.theme.errorContainer
		foreground = s.theme.onErrorContainer
	}
	gtx.Constraints.Min.Y = gtx.Dp(32)
	return s.roundedSurface(gtx, shapeLarge, background, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 6, Bottom: 6, Left: 12, Right: 12}.Layout(gtx,
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
	case strings.Contains(lower, "mcp settings unavailable"):
		return "MCP unavailable"
	case strings.Contains(lower, "history failed"):
		return "History failed"
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

func (s *shell) layoutSidebar(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	width := unit.Dp(288)
	if gtx.Constraints.Max.X < gtx.Dp(900) {
		width = 248
	}
	gtx.Constraints.Min.X = gtx.Dp(width)
	gtx.Constraints.Max.X = gtx.Dp(width)
	dims := s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainerLow, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutSidebarHeader(gtx, snapshot)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				rows := s.sidebarRows(snapshot.State, snapshot.AgentProfiles)
				if len(rows) == 0 {
					return desktopInset{Top: 24, Bottom: 24, Left: 20, Right: 20}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "No conversations yet. Start one from an existing workspace.", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 3)
						},
					)
				}
				return s.sidebarList.Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
					return s.layoutSidebarRow(gtx, rows[index], snapshot.State)
				})
			}),
		)
	})
	return widget.Border{Color: s.theme.outlineVariant, CornerRadius: shapeLarge, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return dims
	})
}

func (s *shell) layoutSidebarHeader(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(64)
	return desktopInset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, "Conversations", textLabelLarge, font.SemiBold, s.theme.onSurfaceVariant, 1)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := "+ New"
					if snapshot.CreatingSession {
						label = "Creating…"
					}
					return s.layoutPrimaryButton(gtx, &s.sidebarNewSessionButton, label, snapshot.Connection == connectionConnected && !snapshot.CreatingSession, s.onNewSession)
				}),
			)
		},
	)
}

func (s *shell) sidebarRows(state desktopstate.State, profiles []app.ACPAgentProfile) []sidebarRow {
	if s.sidebarRowsCache.valid && s.sidebarRowsCache.matches(state, profiles) {
		return s.sidebarRowsCache.rows
	}
	rows, cache := buildSidebarRows(state, profiles)
	s.sidebarRowsCache = cache
	return rows
}

func buildSidebarRows(state desktopstate.State, profiles []app.ACPAgentProfile) ([]sidebarRow, sidebarRowsCache) {
	rows := make([]sidebarRow, 0, len(state.Sessions)+len(state.Projects))
	projects := make([]sidebarProjectCache, 0, len(state.Projects))
	sessions := make([]sidebarSessionCache, 0, len(state.Sessions))
	sessionsByProject := make(map[string][]sidebarRow, len(state.Sessions))
	for _, session := range state.Sessions {
		subtitle := agentDisplayName(profiles, session.AgentID)
		sessions = append(sessions, sidebarSessionCache{
			id:             session.ID,
			projectID:      session.ProjectID,
			agentID:        session.AgentID,
			title:          session.Title,
			subtitle:       subtitle,
			status:         string(session.Status),
			lastActivityAt: session.LastActivityAt,
		})
		sessionsByProject[session.ProjectID] = append(sessionsByProject[session.ProjectID], sidebarRow{
			Kind:           sidebarSessionRow,
			ProjectID:      session.ProjectID,
			SessionID:      session.ID,
			AgentID:        session.AgentID,
			Title:          session.Title,
			Subtitle:       subtitle,
			Status:         displayStatus(session.Status),
			LastActivityAt: session.LastActivityAt,
		})
	}
	for _, project := range state.Projects {
		projects = append(projects, sidebarProjectCache{id: project.ID, name: project.Name})
		projectSessions := sessionsByProject[project.ID]
		sort.SliceStable(projectSessions, func(i, j int) bool {
			left, right := projectSessions[i].LastActivityAt, projectSessions[j].LastActivityAt
			if left.IsZero() {
				return false
			}
			if right.IsZero() {
				return true
			}
			return left.After(right)
		})
		rows = append(rows, sidebarRow{
			Kind:         sidebarProjectRow,
			ProjectID:    project.ID,
			Title:        project.Name,
			SessionCount: len(projectSessions),
		})
		rows = append(rows, projectSessions...)
	}
	return rows, sidebarRowsCache{valid: true, rows: rows, projects: projects, sessions: sessions}
}

func (cache sidebarRowsCache) matches(state desktopstate.State, profiles []app.ACPAgentProfile) bool {
	if len(cache.projects) != len(state.Projects) || len(cache.sessions) != len(state.Sessions) {
		return false
	}
	projectIndex := 0
	for _, project := range state.Projects {
		if projectIndex >= len(cache.projects) {
			return false
		}
		cachedProject := cache.projects[projectIndex]
		if cachedProject.id != project.ID || cachedProject.name != project.Name {
			return false
		}
		projectIndex++
	}
	if projectIndex != len(cache.projects) || len(state.Sessions) != len(cache.sessions) {
		return false
	}
	for sessionIndex, session := range state.Sessions {
		cachedSession := cache.sessions[sessionIndex]
		if cachedSession.id != session.ID ||
			cachedSession.projectID != session.ProjectID ||
			cachedSession.agentID != session.AgentID ||
			cachedSession.title != session.Title ||
			cachedSession.subtitle != agentDisplayName(profiles, session.AgentID) ||
			cachedSession.status != string(session.Status) ||
			!cachedSession.lastActivityAt.Equal(session.LastActivityAt) {
			return false
		}
	}
	return true
}

func (s *shell) layoutSidebarRow(gtx layout.Context, row sidebarRow, state desktopstate.State) layout.Dimensions {
	if row.Kind == sidebarProjectRow {
		button := s.projectButtons[row.ProjectID]
		selected := state.ActiveProjectID == row.ProjectID && state.ActiveSessionID == ""
		if button.Clicked(gtx) {
			s.onSelectProject(row.ProjectID)
		}
		gtx.Constraints.Min.Y = gtx.Dp(44)
		dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(44)
			semantic.Button.Add(gtx.Ops)
			semantic.SelectedOp(selected).Add(gtx.Ops)
			semantic.DescriptionOp(fmt.Sprintf("Select workspace %s, %d conversations", row.Title, row.SessionCount)).Add(gtx.Ops)
			background := s.theme.surfaceContainer
			foreground := s.theme.onSurface
			if selected {
				background = s.theme.primaryContainer
				foreground = s.theme.onPrimaryContainer
			} else if button.Hovered() {
				background = s.theme.surfaceContainerHigh
			}
			return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					children := []layout.FlexChild{
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, row.Title, textLabelLarge, font.SemiBold, foreground, 1)
						}),
					}
					if row.SessionCount > 0 {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, fmt.Sprintf("%d", row.SessionCount), textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
						}))
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
				})
			})
		})
		if gtx.Focused(button) {
			widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: dims.Size}
			})
		}
		return desktopInset{Top: 8, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}

	return desktopInset{Left: 12, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		button := s.sessionButtons[row.SessionID]
		selected := state.ActiveSessionID == row.SessionID
		if button.Clicked(gtx) {
			s.onSelectSession(row.SessionID)
		}
		gtx.Constraints.Min.Y = gtx.Dp(56)
		subtitle := sidebarSessionSubtitle(row, gtx.Now)
		statusLabel := sidebarStatusLabel(row.Status)
		dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(56)
			semantic.Button.Add(gtx.Ops)
			semantic.SelectedOp(selected).Add(gtx.Ops)
			description := row.Title
			if subtitle != "" {
				description += ", " + subtitle
			}
			if statusLabel != "" {
				description += ", " + statusLabel
			}
			semantic.DescriptionOp(description).Add(gtx.Ops)
			background := s.theme.surfaceContainerLowest
			foreground := s.theme.onSurface
			borderColor := s.theme.outlineVariant
			if selected {
				background = s.theme.primaryContainer
				foreground = s.theme.onPrimaryContainer
				borderColor = s.theme.primary
			} else if button.Hovered() {
				background = s.theme.surfaceContainerHigh
			}
			return s.roundedBorderSurface(gtx, shapeMedium, background, borderColor, 1, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								subColor := s.theme.onSurfaceVariant
								if selected {
									subColor = s.theme.onPrimaryContainer
								}
								if subtitle == "" {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return s.layoutLabel(gtx, row.Title, textBodyMedium, font.Medium, foreground, 1)
										}),
									)
								}
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, row.Title, textBodyMedium, font.Medium, foreground, 1)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, subtitle, textLabelSmall, font.Normal, subColor, 1)
									}),
								)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if statusLabel == "" {
									return layout.Dimensions{}
								}
								return desktopInset{Left: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutTaskStatus(gtx, statusLabel, taskStatusFromLabel(row.Status))
								})
							}),
						)
					},
				)
			})
		})
		if gtx.Focused(button) {
			widget.Border{Color: s.theme.primary, CornerRadius: shapeMedium, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: dims.Size}
			})
		}
		return dims
	})
}

func (s *shell) layoutMain(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	session, ok := selectedSession(snapshot.State)
	if !ok {
		return s.roundedSurface(gtx, shapeLarge, s.theme.surface, func(gtx layout.Context) layout.Dimensions {
			return s.layoutMainEmptyState(gtx, snapshot)
		})
	}
	wideInspector := gtx.Constraints.Max.X >= gtx.Dp(inspectorWideBreakpoint)
	showInspector := s.shouldShowInspector(wideInspector)
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSessionHeader(gtx, session, snapshot, wideInspector, showInspector)
		}),
	}
	if showInspector && wideInspector {
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return s.layoutConversationPane(gtx, session, snapshot)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutInspector(gtx, session, snapshot, false)
				}),
			)
		}))
	} else if showInspector {
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutInspector(gtx, session, snapshot, true)
		}))
	} else {
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutConversationPane(gtx, session, snapshot)
		}))
	}
	return s.roundedSurface(gtx, shapeLarge, s.theme.surface, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (s *shell) layoutConversationPane(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot) layout.Dimensions {
	children := make([]layout.FlexChild, 0, 3)
	if permission := activePermission(snapshot.State, session.ID); permission != nil {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutPermissionPanel(gtx, *permission)
		}))
	}
	children = append(children,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutConversation(gtx, session, snapshot.HistoryState)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutComposer(gtx, session, snapshot)
		}),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) layoutSessionHeader(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot, wideInspector, showInspector bool) layout.Dimensions {
	gtx.Constraints.Min.Y = gtx.Dp(76)
	return s.roundedBorderSurface(gtx, shapeLarge, s.theme.surfaceContainer, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return desktopInset{Top: 12, Bottom: 12, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if gtx.Constraints.Max.X < gtx.Dp(640) {
				return s.layoutCompactSessionHeader(gtx, session, snapshot, showInspector)
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return s.layoutSessionHeaderIdentity(gtx, session, snapshot)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutSessionHeaderActions(gtx, session, wideInspector, showInspector)
				}),
			)
		})
	})
}

func (s *shell) layoutCompactSessionHeader(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot, showInspector bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSessionHeaderIdentity(gtx, session, snapshot)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSessionHeaderActions(gtx, session, false, showInspector)
		}),
	)
}

func (s *shell) layoutSessionHeaderIdentity(gtx layout.Context, session desktopstate.SessionState, snapshot controllerSnapshot) layout.Dimensions {
	workspace := session.Workspace
	if strings.TrimSpace(workspace) == "" {
		workspace = session.WorkspaceName
	}
	agentID := session.AgentID
	if strings.TrimSpace(agentID) == "" {
		agentID = controllerAgentID
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, session.Title, textHeadlineSmall, font.SemiBold, s.theme.onSurface, 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, workspace+" · "+agentDisplayName(snapshot.AgentProfiles, agentID), textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 1)
		}),
	)
}

func (s *shell) layoutSessionHeaderActions(gtx layout.Context, session desktopstate.SessionState, wideInspector, showInspector bool) layout.Dimensions {
	label := "Inspector"
	if wideInspector && showInspector {
		label = "Hide inspector"
	} else if !wideInspector && showInspector {
		label = "Conversation"
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(36)
			return s.layoutTaskStatus(gtx, displayStatus(session.Status), session.Status)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return desktopUniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutButton(gtx, &s.inspectorToggle, label, true, func() {
					s.inspectorOverride = true
					s.inspectorVisible = !showInspector
				})
			})
		}),
	)
}

func (s *shell) layoutTaskStatus(gtx layout.Context, label string, status desktopstate.TaskStatus) layout.Dimensions {
	background, foreground := s.taskStatusColors(status)
	return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 5, Bottom: 5, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, label, textLabelMedium, font.Medium, foreground, 1)
		})
	})
}

func (s *shell) taskStatusColors(status desktopstate.TaskStatus) (color.NRGBA, color.NRGBA) {
	switch status {
	case desktopstate.TaskRunning:
		return s.theme.primaryContainer, s.theme.onPrimaryContainer
	case desktopstate.TaskWaitingPermission:
		return s.theme.warningContainer, s.theme.onWarningContainer
	case desktopstate.TaskCompleted:
		return s.theme.successContainer, s.theme.onSuccessContainer
	case desktopstate.TaskFailed:
		return s.theme.errorContainer, s.theme.onErrorContainer
	default:
		return s.theme.secondaryContainer, s.theme.onSecondaryContainer
	}
}

func taskStatusFromLabel(label string) desktopstate.TaskStatus {
	return desktopstate.TaskStatus(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(label)), " ", "_"))
}

func (s *shell) layoutMainEmptyState(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	title := "Select a conversation"
	body := "Choose a conversation from the workspace list to view its messages and controls."
	if len(snapshot.State.Sessions) == 0 {
		title = "Start a conversation"
		body = "Choose an agent, then start a conversation in an existing workspace. Your conversations stay with the agent that started them."
	}
	return s.layoutCenteredCard(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, title, textDisplaySmall, font.SemiBold, s.theme.onSurface, 2)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopUniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, body, textBodyLarge, font.Normal, s.theme.onSurfaceVariant, 4)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(snapshot.State.Sessions) != 0 {
					return layout.Dimensions{}
				}
				label := "New conversation"
				if snapshot.CreatingSession {
					label = "Creating…"
				}
				return desktopUniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutPrimaryButton(gtx, &s.emptyNewSessionButton, label, snapshot.Connection == connectionConnected && !snapshot.CreatingSession, s.onNewSession)
				})
			}),
		)
	})
}

func (s *shell) layoutSessionDetails(gtx layout.Context, session desktopstate.SessionState) layout.Dimensions {
	return s.layoutCenteredCard(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, "Session overview", textTitleMedium, font.SemiBold, s.theme.onSurface, 1)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return desktopUniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.layoutLabel(gtx, "Your conversation, permissions, and session controls appear here.", textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 4)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutDetailRow(gtx, "Workspace", session.Workspace)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutDetailRow(gtx, "Workspace key", session.WorkspaceKey)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutDetailRow(gtx, "Agent", session.AgentID)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutDetailRow(gtx, "Session ID", session.ID)
			}),
		)
	})
}

func (s *shell) layoutDetailRow(gtx layout.Context, label, value string) layout.Dimensions {
	if value == "" {
		value = "Not reported"
	}
	if gtx.Constraints.Max.X-gtx.Constraints.Min.X < gtx.Dp(480) {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, label, textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 1)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, value, textBodyMedium, font.Normal, s.theme.onSurface, 2)
			}),
		)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(112)
			return s.layoutLabel(gtx, label, textLabelMedium, font.SemiBold, s.theme.onSurfaceVariant, 2)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, value, textBodyMedium, font.Normal, s.theme.onSurface, 2)
		}),
	)
}

func (s *shell) layoutCenteredCard(gtx layout.Context, content layout.Widget) layout.Dimensions {
	gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(680))
	gtx.Constraints.Min.X = 0
	return layout.Stack{Alignment: layout.Center}.Layout(gtx, layout.Stacked(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(680))
		return s.roundedSurface(gtx, shapeExtraLarge, s.theme.surfaceContainer, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 24, Bottom: 24, Left: 28, Right: 28}.Layout(gtx, content)
		})
	}))
}

func (s *shell) layoutButton(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func()) layout.Dimensions {
	return s.layoutButtonStyle(gtx, button, label, enabled, action, false, false)
}

func (s *shell) layoutPrimaryButton(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func()) layout.Dimensions {
	return s.layoutButtonStyle(gtx, button, label, enabled, action, true, false)
}

func (s *shell) layoutDangerButton(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func()) layout.Dimensions {
	return s.layoutButtonStyle(gtx, button, label, enabled, action, false, true)
}

func (s *shell) layoutButtonStyle(gtx layout.Context, button *widget.Clickable, label string, enabled bool, action func(), primary, danger bool) layout.Dimensions {
	if enabled && button.Clicked(gtx) && action != nil {
		action()
	}
	if !enabled {
		gtx = gtx.Disabled()
	}
	gtx.Constraints.Min.Y = gtx.Dp(44)
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(44)
		semantic.Button.Add(gtx.Ops)
		semantic.EnabledOp(gtx.Enabled()).Add(gtx.Ops)
		semantic.DescriptionOp(label).Add(gtx.Ops)
		background := s.theme.secondaryContainer
		foreground := s.theme.onSecondaryContainer
		if primary {
			background = s.theme.primary
			foreground = s.theme.onPrimary
		} else if danger {
			background = s.theme.errorContainer
			foreground = s.theme.onErrorContainer
		}
		if !gtx.Enabled() {
			background = s.theme.surfaceContainerHigh
			foreground = s.theme.onSurfaceVariant
		} else if button.Hovered() && !primary && !danger {
			background = s.theme.primaryContainer
			foreground = s.theme.onPrimaryContainer
		}
		return s.roundedSurface(gtx, shapeMedium, background, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 10, Bottom: 10, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, label, textLabelLarge, font.SemiBold, foreground, 1)
			})
		})
	})
	if enabled && gtx.Focused(button) {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeMedium, Width: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *shell) layoutLabel(gtx layout.Context, value string, size unit.Sp, weight font.Weight, color color.NRGBA, maxLines int) layout.Dimensions {
	material := op.Record(gtx.Ops)
	paint.ColorOp{Color: color}.Add(gtx.Ops)
	return widget.Label{MaxLines: maxLines}.Layout(gtx, s.theme.material.Shaper, s.theme.textFont(weight), size, value, material.Stop())
}

func (s *shell) roundedBorderSurface(gtx layout.Context, radius unit.Dp, background color.NRGBA, borderColor color.NRGBA, borderWidth int, content layout.Widget) layout.Dimensions {
	dims := s.roundedSurface(gtx, radius, background, content)
	if borderWidth > 0 {
		widget.Border{Color: borderColor, CornerRadius: radius, Width: unit.Dp(borderWidth)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	return dims
}

func (s *shell) roundedSurface(gtx layout.Context, radius unit.Dp, background color.NRGBA, content layout.Widget) layout.Dimensions {
	material := op.Record(gtx.Ops)
	dims := content(gtx)
	call := material.Stop()
	cornerRadius := min(gtx.Dp(radius), dims.Size.X/2, dims.Size.Y/2)
	stack := clip.RRect{
		Rect: image.Rectangle{Max: dims.Size},
		SE:   cornerRadius,
		SW:   cornerRadius,
		NW:   cornerRadius,
		NE:   cornerRadius,
	}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, background)
	call.Add(gtx.Ops)
	stack.Pop()
	return dims
}

func (s *shell) syncSessionButtons(state desktopstate.State, revision uint64) {
	if revision != 0 && s.buttonRevisionSet && s.buttonRevision == revision {
		return
	}
	liveProjects := s.projectButtonLive
	if liveProjects == nil {
		liveProjects = make(map[string]struct{}, len(state.Projects))
		s.projectButtonLive = liveProjects
	}
	clear(liveProjects)
	for _, project := range state.Projects {
		liveProjects[project.ID] = struct{}{}
		if s.projectButtons[project.ID] == nil {
			s.projectButtons[project.ID] = new(widget.Clickable)
		}
	}
	for projectID := range s.projectButtons {
		if _, ok := liveProjects[projectID]; !ok {
			delete(s.projectButtons, projectID)
		}
	}
	live := s.sessionButtonLive
	if live == nil {
		live = make(map[string]struct{}, len(state.Sessions))
		s.sessionButtonLive = live
	}
	clear(live)
	for _, session := range state.Sessions {
		live[session.ID] = struct{}{}
		if s.sessionButtons[session.ID] == nil {
			s.sessionButtons[session.ID] = new(widget.Clickable)
		}
	}
	for sessionID := range s.sessionButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionButtons, sessionID)
		}
	}
	if revision != 0 {
		s.buttonRevision = revision
		s.buttonRevisionSet = true
	}
}

func selectedSession(state desktopstate.State) (desktopstate.SessionState, bool) {
	for _, session := range state.Sessions {
		if session.ID == state.ActiveSessionID {
			return session, true
		}
	}
	return desktopstate.SessionState{}, false
}

func displayStatus(status desktopstate.TaskStatus) string {
	value := strings.ReplaceAll(strings.TrimSpace(string(status)), "_", " ")
	if value == "" {
		return "idle"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func sidebarStatusLabel(status string) string {
	status = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(status)), "_", " ")
	switch status {
	case "", "idle":
		return ""
	case "waiting permission":
		return "Approval"
	case "waiting user":
		return "Your input"
	default:
		return strings.ToUpper(status[:1]) + status[1:]
	}
}

func sidebarSessionSubtitle(row sidebarRow, now time.Time) string {
	if !row.LastActivityAt.IsZero() {
		return sidebarActivityLabel(row.LastActivityAt, now)
	}
	if row.AgentID != "" && row.AgentID != controllerAgentID {
		return row.Subtitle
	}
	return ""
}

func sidebarActivityLabel(activity, now time.Time) string {
	ago := now.Sub(activity)
	if ago < time.Minute {
		return "Just now"
	}
	if ago < time.Hour {
		return fmt.Sprintf("%dm ago", max(1, int(ago.Minutes())))
	}
	if ago < 24*time.Hour {
		return fmt.Sprintf("%dh ago", max(1, int(ago.Hours())))
	}
	if ago < 7*24*time.Hour {
		return fmt.Sprintf("%dd ago", max(1, int(ago.Hours()/24)))
	}
	return activity.Local().Format("Jan 2")
}
