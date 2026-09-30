//go:build desktop || desktop_gio

package gioui

import (
	"fmt"
	"image"
	"image/color"
	"os/exec"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// This file owns sidebar presentation: row model and filtering, header,
// search, row rendering, footer, session actions, and sidebar-only helpers.

type sidebarRowKind uint8

const (
	sidebarProjectRow sidebarRowKind = iota
	sidebarSessionRow
	sidebarPinnedHeaderRow
)

type sidebarRow struct {
	Kind           sidebarRowKind
	ProjectID      string
	SessionID      string
	sessionKey     string
	AgentID        string
	Title          string
	Subtitle       string
	Status         string
	SessionCount   int
	LastActivityAt time.Time
	Pinned         bool
}

type sidebarProjectCache struct {
	id   string
	name string
}

type sidebarSessionCache struct {
	id             string
	key            string
	projectID      string
	agentID        string
	title          string
	subtitle       string
	status         string
	lastActivityAt time.Time
	pinned         bool
}

type sidebarRowsCache struct {
	valid      bool
	revision   uint64
	rows       []sidebarRow
	projects   []sidebarProjectCache
	sessions   []sidebarSessionCache
	filterMode string
	pinned     []string
}

type sidebarDisplayCache struct {
	valid            bool
	rows             []sidebarRow
	sourceRevision   uint64
	collapseRevision uint64
	query            string
	filterMode       string
	pinnedCollapsed  bool
}

// Sidebar vertical rhythm and glyph sizing. These keep row heights stable as
// status metadata appears and disappears, and keep icon sizes on one scale.
const (
	sidebarSessionRowMinHeight = 44
	sidebarRowGlyphSize        = 14
	sidebarMetadataLineHeight  = 15
	sidebarRowIndent           = 22
	sidebarGroupTopInset       = 8
	sidebarGroupBottomInset    = 2
)

func (s *shell) layoutSidebar(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	width := unit.Dp(260)
	if gtx.Constraints.Max.X < gtx.Dp(900) {
		width = 220
	}
	gtx.Constraints.Min.X = gtx.Dp(width)
	gtx.Constraints.Max.X = gtx.Dp(width)
	paint.FillShape(gtx.Ops, s.theme.surfaceDim, clip.Rect{
		Max: image.Pt(gtx.Dp(width), gtx.Constraints.Max.Y),
	}.Op())
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSidebarHeader(gtx, snapshot)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			allRows := s.sidebarRows(snapshot)
			rows := s.sidebarDisplayRows(allRows, snapshot.FilterMode, snapshot.Revision)
			if len(rows) == 0 {
				emptyText := "No conversations yet. Start one from an existing workspace."
				hasFilter := false
				if strings.TrimSpace(s.sidebarSearchEditor.Text()) != "" {
					emptyText = "No conversations matching \"" + s.sidebarSearchEditor.Text() + "\""
					hasFilter = true
				} else if snapshot.FilterMode == "running" {
					emptyText = "No running conversations."
					hasFilter = true
				} else if snapshot.FilterMode == "pinned" {
					emptyText = "No pinned conversations. Click ⋮ on a session to pin it."
					hasFilter = true
				}
				if s.sidebarClearFilterButton.Clicked(gtx) {
					s.sidebarSearchEditor.SetText("")
					if s.onSetFilterMode != nil {
						s.onSetFilterMode("all")
					}
				}
				return desktopInset{Top: 24, Bottom: 24, Left: 16, Right: 16}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						children := []layout.FlexChild{
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, emptyText, textBodySmall, font.Normal, s.theme.onSurfaceVariant, 3)
							}),
						}
						if hasFilter {
							children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Top: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutButton(gtx, &s.sidebarClearFilterButton, "Clear filter", true, nil)
								})
							}))
						}
						return layout.Flex{Axis: layout.Vertical, Alignment: layout.Start}.Layout(gtx, children...)
					},
				)
			}
			return s.sidebarList.Layout(gtx, len(rows), func(gtx layout.Context, index int) layout.Dimensions {
				return s.layoutSidebarRow(gtx, rows[index], snapshot.State)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutHorizontalDivider(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.layoutSidebarFooter(gtx, snapshot)
		}),
	)
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

func (s *shell) layoutSidebarFooter(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	if s.sidebarInspectorButton.Clicked(gtx) {
		s.openSettingsModal()
		gtx.Execute(key.FocusCmd{Tag: nil})
	}

	if s.sidebarPinnedFilterButton.Clicked(gtx) && s.onSetFilterMode != nil {
		if snapshot.FilterMode == "pinned" {
			s.onSetFilterMode("all")
		} else {
			s.onSetFilterMode("pinned")
		}
	}

	if s.sidebarCommunityButton.Clicked(gtx) {
		openBrowserURL("https://github.com/phongsathornpt/protonman")
	}

	statusColor := s.theme.onSuccessContainer
	statusLabel := "Connected"
	switch snapshot.Connection {
	case connectionConnected:
		statusColor = s.theme.onSuccessContainer
		statusLabel = "Connected"
	case connectionConnecting, connectionReconnecting:
		statusColor = s.theme.onWarningContainer
		statusLabel = "Connecting"
	default:
		statusColor = s.theme.onErrorContainer
		statusLabel = "Offline"
	}
	status := strings.ToLower(snapshot.Status)
	if strings.Contains(status, "failed") || (strings.Contains(status, "unavailable") && !strings.Contains(status, "session list unavailable")) || strings.Contains(status, "disconnected") {
		statusColor = s.theme.onErrorContainer
		statusLabel = "Offline"
	}

	return desktopInset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btnGtx := gtx
						bg := color.NRGBA{}
						fg := s.theme.onSurfaceVariant
						if s.settingsModalOpen {
							bg = s.theme.primaryContainer
							fg = s.theme.onPrimaryContainer
						} else if s.sidebarInspectorButton.Hovered() {
							bg = s.theme.surfaceContainerHigh
							fg = s.theme.onSurface
						}
						semantic.Button.Add(btnGtx.Ops)
						semantic.DescriptionOp("Open Settings (⌘,)").Add(btnGtx.Ops)
						return s.sidebarInspectorButton.Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
							return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
								return desktopUniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, iconSettings, 16, fg)
								})
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btnGtx := gtx
							bg := color.NRGBA{}
							fg := s.theme.onSurfaceVariant
							if snapshot.FilterMode == "pinned" {
								bg = s.theme.primaryContainer
								fg = s.theme.onPrimaryContainer
							} else if s.sidebarPinnedFilterButton.Hovered() {
								bg = s.theme.surfaceContainerHigh
								fg = s.theme.onSurface
							}
							semantic.Button.Add(btnGtx.Ops)
							semantic.DescriptionOp("Toggle pinned conversations filter").Add(btnGtx.Ops)
							return s.sidebarPinnedFilterButton.Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
								return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
									return desktopUniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutActionIcon(gtx, iconStar, 16, fg)
									})
								})
							})
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btnGtx := gtx
							bg := color.NRGBA{}
							fg := s.theme.onSurfaceVariant
							if s.sidebarCommunityButton.Hovered() {
								bg = s.theme.surfaceContainerHigh
								fg = s.theme.onSurface
							}
							semantic.Button.Add(btnGtx.Ops)
							semantic.DescriptionOp("Open Protonman Community").Add(btnGtx.Ops)
							return s.sidebarCommunityButton.Layout(btnGtx, func(gtx layout.Context) layout.Dimensions {
								return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
									return desktopUniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutActionIcon(gtx, iconDiscord, 16, fg)
									})
								})
							})
						})
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.roundedSurface(gtx, shapeFull, statusColor, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min = image.Pt(gtx.Dp(6), gtx.Dp(6))
							gtx.Constraints.Max = image.Pt(gtx.Dp(6), gtx.Dp(6))
							return layout.Dimensions{Size: gtx.Constraints.Min}
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return desktopInset{Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, statusLabel, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
						})
					}),
				)
			}),
		)
	})
}

func (s *shell) layoutSidebarHeader(gtx layout.Context, snapshot controllerSnapshot) layout.Dimensions {
	return desktopInset{Top: 14, Bottom: 4, Left: 12, Right: 12}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			newLabel := "New chat"
			if snapshot.CreatingSession {
				newLabel = "Starting…"
			}
			canCreate := snapshot.Connection == connectionConnected && !snapshot.CreatingSession

			items := []layout.FlexChild{
				// Brand Header: [P] Protonman
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 14, Left: 4, Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return s.layoutActionIcon(gtx, iconBrandLogo, 20, s.theme.onSurface)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return s.layoutLabel(gtx, "Protonman", textTitleMedium, font.Bold, s.theme.onSurface, 1)
							}),
						)
					})
				}),
				// "New chat" dedicated action row
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := &s.sidebarNewSessionButton
					if btn.Clicked(gtx) && canCreate && s.onNewSession != nil {
						s.onNewSession()
					}
					bg := color.NRGBA{}
					fg := s.theme.onSurface
					if btn.Hovered() && canCreate {
						bg = s.theme.surfaceContainerHigh
					}
					semantic.Button.Add(gtx.Ops)
					semantic.DescriptionOp("Start a new chat session").Add(gtx.Ops)
					return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.roundedSurface(gtx, shapeSmall, bg, func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 7, Bottom: 7, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutActionIcon(gtx, iconCompose, 16, fg)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutLabel(gtx, newLabel, textBodyMedium, font.Medium, fg, 1)
									}),
								)
							})
						})
					})
				}),
				// Full-width search bar: Search threads
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Top: 6, Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutSidebarSearch(gtx)
					})
				}),
				// "Projects" section label
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Bottom: 6, Left: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutLabel(gtx, "Projects", textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
					})
				}),
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
		},
	)
}

func (s *shell) layoutSidebarSearch(gtx layout.Context) layout.Dimensions {
	for {
		evt, ok := gtx.Event(key.Filter{Focus: &s.sidebarSearchEditor, Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := evt.(key.Event); ok && e.State == key.Press {
			s.sidebarSearchEditor.SetText("")
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}
	if s.sidebarSearchClearButton.Clicked(gtx) {
		s.sidebarSearchEditor.SetText("")
	}

	gtx.Constraints.Min.Y = gtx.Dp(28)
	isFocused := gtx.Focused(&s.sidebarSearchEditor)
	borderColor := color.NRGBA{}
	borderWidth := 0
	iconColor := s.theme.onSurfaceVariant
	if isFocused {
		borderColor = s.theme.primary
		borderWidth = 1
		iconColor = s.theme.primary
	}

	return s.roundedBorderSurface(gtx, shapeMedium, s.theme.surfaceContainerHigh, borderColor, borderWidth, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.layoutActionIcon(gtx, iconSearch, 16, iconColor)
					})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(s.theme.material, &s.sidebarSearchEditor, "Search threads…")
					ed.TextSize = textBodySmall
					ed.Color = s.theme.onSurface
					ed.HintColor = s.theme.onSurfaceVariant
					return ed.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if s.sidebarSearchEditor.Text() == "" {
						return layout.Dimensions{}
					}
					return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						semantic.Button.Add(gtx.Ops)
						semantic.DescriptionOp("Clear conversation search").Add(gtx.Ops)
						return s.layoutMiniIconButton(gtx, &s.sidebarSearchClearButton, "×", s.theme.onSurfaceVariant)
					})
				}),
			)
		})
	})
}

func (s *shell) sidebarDisplayRows(rows []sidebarRow, filterMode string, sourceRevisions ...uint64) []sidebarRow {
	query := strings.ToLower(strings.TrimSpace(s.sidebarSearchEditor.Text()))
	filterMode = strings.ToLower(strings.TrimSpace(filterMode))
	sourceRevision := uint64(0)
	if len(sourceRevisions) > 0 {
		sourceRevision = sourceRevisions[0]
	}
	cache := &s.sidebarDisplayCache
	if cache.valid && cache.sourceRevision == sourceRevision && cache.collapseRevision == s.sidebarCollapseRevision &&
		cache.query == query && cache.filterMode == filterMode && cache.pinnedCollapsed == s.pinnedCollapsed {
		return cache.rows
	}

	display := make([]sidebarRow, 0, len(rows))
	currentProjectCollapsed := false
	inPinnedSection := false

	matchesFilter := func(row sidebarRow) bool {
		switch filterMode {
		case "running":
			statusLower := strings.ToLower(row.Status)
			return strings.Contains(statusLower, "running") ||
				strings.Contains(statusLower, "permission") ||
				strings.Contains(statusLower, "approval")
		case "pinned":
			return row.Pinned
		default:
			return true
		}
	}

	matchesQuery := func(row sidebarRow) bool {
		if query == "" {
			return true
		}
		return strings.Contains(strings.ToLower(row.Title), query) ||
			strings.Contains(strings.ToLower(row.Subtitle), query) ||
			strings.Contains(strings.ToLower(row.ProjectID), query)
	}

	for _, row := range rows {
		switch row.Kind {
		case sidebarPinnedHeaderRow:
			inPinnedSection = true
			if filterMode != "running" {
				display = append(display, row)
			}
		case sidebarProjectRow:
			inPinnedSection = false
			currentProjectCollapsed = s.projectCollapsed[row.ProjectID]
			display = append(display, row)
		case sidebarSessionRow:
			if inPinnedSection {
				if s.pinnedCollapsed {
					continue
				}
				if matchesFilter(row) && matchesQuery(row) {
					display = append(display, row)
				}
			} else {
				if currentProjectCollapsed {
					continue
				}
				if matchesFilter(row) && matchesQuery(row) {
					display = append(display, row)
				}
			}
		}
	}

	if query != "" || filterMode == "running" || filterMode == "pinned" {
		cleaned := make([]sidebarRow, 0, len(display))
		for i, r := range display {
			if r.Kind == sidebarProjectRow || r.Kind == sidebarPinnedHeaderRow {
				hasSessions := false
				for j := i + 1; j < len(display); j++ {
					if display[j].Kind == sidebarProjectRow || display[j].Kind == sidebarPinnedHeaderRow {
						break
					}
					if display[j].Kind == sidebarSessionRow {
						hasSessions = true
						break
					}
				}
				if hasSessions {
					cleaned = append(cleaned, r)
				}
			} else {
				cleaned = append(cleaned, r)
			}
		}
		display = cleaned
	}

	cache.valid = true
	cache.rows = display
	cache.sourceRevision = sourceRevision
	cache.collapseRevision = s.sidebarCollapseRevision
	cache.query = query
	cache.filterMode = filterMode
	cache.pinnedCollapsed = s.pinnedCollapsed
	return display
}

func (s *shell) sidebarRows(snapshot controllerSnapshot) []sidebarRow {
	if snapshot.Revision != 0 && s.sidebarRowsCache.valid && s.sidebarRowsCache.revision == snapshot.Revision {
		return s.sidebarRowsCache.rows
	}
	if s.sidebarRowsCache.valid && s.sidebarRowsCache.matchesWithOptions(snapshot.State, snapshot.AgentProfiles, snapshot.FilterMode, snapshot.PinnedSessions, snapshot.CustomTitles) {
		return s.sidebarRowsCache.rows
	}
	rows, cache := buildSidebarRowsWithOptions(snapshot.State, snapshot.AgentProfiles, snapshot.PinnedSessions, snapshot.CustomTitles)
	cache.filterMode = snapshot.FilterMode
	cache.revision = snapshot.Revision
	s.sidebarRowsCache = cache
	return rows
}

func buildSidebarRows(state desktopstate.State, profiles []app.ACPAgentProfile) ([]sidebarRow, sidebarRowsCache) {
	return buildSidebarRowsWithOptions(state, profiles, nil, nil)
}

func buildSidebarRowsWithOptions(state desktopstate.State, profiles []app.ACPAgentProfile, pinnedSessions []string, customTitles map[string]string) ([]sidebarRow, sidebarRowsCache) {
	rows := make([]sidebarRow, 0, len(state.Sessions)+len(state.Projects)+1)
	projects := make([]sidebarProjectCache, 0, len(state.Projects))
	sessions := make([]sidebarSessionCache, 0, len(state.Sessions))
	sessionsByProject := make(map[string][]sidebarRow, len(state.Sessions))
	pinnedMap := make(map[string]bool, len(pinnedSessions))
	for _, id := range pinnedSessions {
		pinnedMap[id] = true
	}

	pinnedRows := make([]sidebarRow, 0, len(pinnedSessions))

	displayNames := make(map[string]string)
	subtitleFor := func(agentID string) string {
		if name, ok := displayNames[agentID]; ok {
			return name
		}
		name := agentDisplayName(profiles, agentID)
		displayNames[agentID] = name
		return name
	}

	for _, session := range state.Sessions {
		storageKey := sessionRefStorageKey(session.Ref())
		title := session.Title
		custom, ok := customTitles[storageKey]
		if !ok && storageKey != session.ID {
			custom = customTitles[session.ID]
		}
		if custom != "" {
			title = custom
		}
		pinned := pinnedMap[storageKey] || pinnedMap[session.ID]
		subtitle := subtitleFor(session.AgentID)
		sessions = append(sessions, sidebarSessionCache{
			id:             session.ID,
			key:            storageKey,
			projectID:      session.ProjectID,
			agentID:        session.AgentID,
			title:          title,
			subtitle:       subtitle,
			status:         string(session.Status),
			lastActivityAt: session.LastActivityAt,
			pinned:         pinned,
		})
		row := sidebarRow{
			Kind:           sidebarSessionRow,
			ProjectID:      session.ProjectID,
			SessionID:      session.ID,
			sessionKey:     storageKey,
			AgentID:        session.AgentID,
			Title:          title,
			Subtitle:       subtitle,
			Status:         displayStatus(session.Status),
			LastActivityAt: session.LastActivityAt,
			Pinned:         pinned,
		}
		sessionsByProject[session.ProjectID] = append(sessionsByProject[session.ProjectID], row)
		if pinned {
			pinnedRows = append(pinnedRows, row)
		}
	}

	if len(pinnedRows) > 0 {
		sort.SliceStable(pinnedRows, func(i, j int) bool {
			left, right := pinnedRows[i].LastActivityAt, pinnedRows[j].LastActivityAt
			if left.IsZero() {
				return false
			}
			if right.IsZero() {
				return true
			}
			return left.After(right)
		})
		rows = append(rows, sidebarRow{
			Kind:         sidebarPinnedHeaderRow,
			Title:        "Pinned",
			SessionCount: len(pinnedRows),
		})
		rows = append(rows, pinnedRows...)
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

	return rows, sidebarRowsCache{
		valid:    true,
		rows:     rows,
		projects: projects,
		sessions: sessions,
		pinned:   slices.Clone(pinnedSessions),
	}
}

func (cache sidebarRowsCache) matches(state desktopstate.State, profiles []app.ACPAgentProfile) bool {
	return cache.matchesWithOptions(state, profiles, cache.filterMode, nil, nil)
}

func (cache sidebarRowsCache) matchesWithOptions(state desktopstate.State, profiles []app.ACPAgentProfile, filterMode string, pinnedSessions []string, customTitles map[string]string) bool {
	if cache.filterMode != filterMode || !slices.Equal(cache.pinned, pinnedSessions) {
		return false
	}
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
	displayNames := make(map[string]string)
	for sessionIndex, session := range state.Sessions {
		cachedSession := cache.sessions[sessionIndex]
		storageKey := cachedSession.key
		expectedTitle := session.Title
		custom, ok := customTitles[storageKey]
		if !ok && storageKey != session.ID {
			custom = customTitles[session.ID]
		}
		if custom != "" {
			expectedTitle = custom
		}
		subtitle, ok := displayNames[session.AgentID]
		if !ok {
			subtitle = agentDisplayName(profiles, session.AgentID)
			displayNames[session.AgentID] = subtitle
		}
		if cachedSession.id != session.ID ||
			cachedSession.projectID != session.ProjectID ||
			cachedSession.agentID != session.AgentID ||
			cachedSession.title != expectedTitle ||
			cachedSession.subtitle != subtitle ||
			cachedSession.status != string(session.Status) ||
			!cachedSession.lastActivityAt.Equal(session.LastActivityAt) {
			return false
		}
	}
	return true
}

func (s *shell) layoutSidebarRow(gtx layout.Context, row sidebarRow, state desktopstate.State) layout.Dimensions {
	if row.Kind == sidebarPinnedHeaderRow {
		button := &s.pinnedButton
		if button.Clicked(gtx) {
			s.pinnedCollapsed = !s.pinnedCollapsed
			s.sidebarCollapseRevision++
		}
		gtx.Constraints.Min.Y = gtx.Dp(32)
		chevronKind := iconChevronDown
		if s.pinnedCollapsed {
			chevronKind = iconChevronRight
		}
		dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(28)
			background := color.NRGBA{}
			foreground := s.theme.onSurfaceVariant
			if button.Hovered() {
				background = s.theme.surfaceContainerHigh
				foreground = s.theme.onSurface
			}
			return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, chevronKind, sidebarRowGlyphSize, s.theme.onSurfaceVariant)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, iconStar, sidebarRowGlyphSize, s.theme.primary)
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Pinned", textLabelSmall, font.SemiBold, foreground, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if row.SessionCount > 0 {
								return s.layoutCountPill(gtx, row.SessionCount)
							}
							return layout.Dimensions{}
						}),
					)
				})
			})
		})
		return desktopInset{Top: sidebarGroupTopInset, Bottom: sidebarGroupBottomInset, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}

	if row.Kind == sidebarProjectRow {
		button := s.projectButtons[row.ProjectID]
		selected := state.ActiveProjectID == row.ProjectID && state.ActiveSessionID == ""
		isCollapsed := s.projectCollapsed[row.ProjectID]
		if button.Clicked(gtx) {
			s.projectCollapsed[row.ProjectID] = !isCollapsed
			s.sidebarCollapseRevision++
			s.onSelectProject(row.ProjectID)
		}
		gtx.Constraints.Min.Y = gtx.Dp(32)
		chevronKind := iconChevronDown
		if isCollapsed {
			chevronKind = iconChevronRight
		}
		dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = gtx.Dp(28)
			semantic.Button.Add(gtx.Ops)
			semantic.SelectedOp(selected).Add(gtx.Ops)
			semantic.DescriptionOp(fmt.Sprintf("Select workspace %s, %d conversations", row.Title, row.SessionCount)).Add(gtx.Ops)
			background := color.NRGBA{}
			foreground := s.theme.onSurfaceVariant
			if selected {
				background = s.theme.surfaceContainerHigh
				foreground = s.theme.onSurface
			} else if button.Hovered() {
				background = s.theme.surfaceContainerHigh
				foreground = s.theme.onSurface
			}
			return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 4, Bottom: 4, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, chevronKind, sidebarRowGlyphSize, s.theme.onSurfaceVariant)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutActionIcon(gtx, iconFolder, sidebarRowGlyphSize, s.theme.onSurfaceVariant)
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, row.Title, textLabelSmall, font.SemiBold, foreground, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if row.SessionCount > 0 {
								return s.layoutCountPill(gtx, row.SessionCount)
							}
							return layout.Dimensions{}
						}),
					)
				})
			})
		})
		if gtx.Focused(button) {
			widget.Border{Color: s.theme.primary, CornerRadius: shapeSmall, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: dims.Size}
			})
		}
		return desktopInset{Top: sidebarGroupTopInset, Bottom: sidebarGroupBottomInset, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}

	widgetKey := row.sessionKey
	if widgetKey == "" {
		widgetKey = sidebarSessionWidgetKey(row.SessionID, row.AgentID)
	}
	button := s.sessionButtons[widgetKey]
	pinBtn := s.sessionPinButton(row.SessionID, row.AgentID)
	renameBtn := s.sessionQuickRenameButton(row.SessionID, row.AgentID)
	deleteBtn := s.sessionQuickDeleteButton(row.SessionID, row.AgentID)
	menuBtn := s.sessionMenuButton(row.SessionID, row.AgentID)
	selected := state.ActiveSessionID == row.SessionID && (state.ActiveAgentID == "" || state.ActiveAgentID == row.AgentID)
	isRenaming := s.editingSessionID == row.SessionID && s.editingSessionAgentID == row.AgentID
	menuOpen := s.menuSessionID == row.SessionID && s.menuSessionAgentID == row.AgentID
	isHovered := button != nil && button.Hovered()
	isFocused := button != nil && gtx.Focused(button)
	showActions := isHovered || selected || menuOpen || isFocused

	if button != nil && button.Clicked(gtx) {
		s.onSelectSession(row.AgentID, row.SessionID)
	}

	if pinBtn.Clicked(gtx) {
		s.onTogglePinSession(row.AgentID, row.SessionID)
	}
	if renameBtn.Clicked(gtx) {
		s.editingSessionID = row.SessionID
		s.editingSessionAgentID = row.AgentID
		s.sessionRenameEditor.SetText(row.Title)
	}
	if deleteBtn.Clicked(gtx) {
		s.deletingSessionID = row.SessionID
		s.deletingSessionAgentID = row.AgentID
		s.deletingSessionTitle = row.Title
	}

	if isRenaming {
		for {
			evt, ok := gtx.Event(key.Filter{Focus: &s.sessionRenameEditor, Name: key.NameReturn})
			if !ok {
				break
			}
			if e, ok := evt.(key.Event); ok && e.State == key.Press {
				newTitle := s.sessionRenameEditor.Text()
				s.editingSessionID = ""
				s.onRenameSession(row.AgentID, row.SessionID, newTitle)
			}
		}
		for {
			evt, ok := gtx.Event(key.Filter{Focus: &s.sessionRenameEditor, Name: key.NameEscape})
			if !ok {
				break
			}
			if e, ok := evt.(key.Event); ok && e.State == key.Press {
				s.editingSessionID = ""
			}
		}
		if s.renameConfirmButton.Clicked(gtx) {
			newTitle := s.sessionRenameEditor.Text()
			s.editingSessionID = ""
			s.onRenameSession(row.AgentID, row.SessionID, newTitle)
		}
		if s.renameCancelButton.Clicked(gtx) {
			s.editingSessionID = ""
		}
	}

	if menuBtn.Clicked(gtx) {
		if s.menuSessionID == row.SessionID && s.menuSessionAgentID == row.AgentID {
			s.menuSessionID = ""
			s.menuSessionAgentID = ""
		} else {
			s.menuSessionID = row.SessionID
			s.menuSessionAgentID = row.AgentID
		}
	}

	gtx.Constraints.Min.Y = gtx.Dp(sidebarSessionRowMinHeight)
	subtitle := sidebarSessionSubtitle(row, gtx.Now)
	statusLabel := sidebarStatusLabel(row.Status)

	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(sidebarSessionRowMinHeight)
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
		background := color.NRGBA{}
		// The title is the row's primary label, so it uses the primary text
		// color; only metadata (subtitle, status) stays muted. Selected is one
		// surface step brighter than hover so the two never read the same.
		foreground := s.theme.onSurface
		if selected {
			background = s.theme.surfaceContainerHighest
		} else if isHovered {
			background = s.theme.surfaceContainerHigh
		}
		return s.roundedSurface(gtx, shapeMedium, background, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 6, Bottom: 6, Left: 12, Right: 8}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					cardChildren := []layout.FlexChild{
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if isRenaming {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										ed := material.Editor(s.theme.material, &s.sessionRenameEditor, "Session title…")
										ed.TextSize = textBodySmall
										ed.Color = foreground
										return ed.Layout(gtx)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutMiniButton(gtx, &s.renameConfirmButton, "✓", s.theme.primary)
										})
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return desktopInset{Left: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutMiniButton(gtx, &s.renameCancelButton, "✕", s.theme.onSurfaceVariant)
										})
									}),
								)
							}
							weight := font.Normal
							if selected {
								weight = font.Medium
							}
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return s.layoutLabel(gtx, row.Title, textBodySmall, weight, foreground, 1)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if row.Pinned && !showActions {
										return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutActionIcon(gtx, iconStar, sidebarRowGlyphSize, s.theme.primary)
										})
									}
									if !showActions {
										return layout.Dimensions{}
									}
									var items []layout.FlexChild
									if row.Pinned {
										items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return desktopInset{Right: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
												return s.layoutActionIcon(gtx, iconStar, sidebarRowGlyphSize, s.theme.primary)
											})
										}))
									}
									items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										return s.layoutMiniMenuButton(gtx, menuBtn, "⋮")
									}))
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, items...)
								}),
							)
						}),
					}
					{
						// The metadata line always renders, even when both the
						// subtitle and status are empty, so a row never changes
						// height as a session moves between idle and running.
						cardChildren = append(cardChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = gtx.Dp(sidebarMetadataLineHeight)
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										if subtitle == "" {
											return layout.Dimensions{}
										}
										return s.layoutLabel(gtx, subtitle, textLabelSmall, font.Normal, s.theme.onSurfaceVariant, 1)
									}),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if statusLabel == "" {
											return layout.Dimensions{}
										}
										return desktopInset{Left: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return s.layoutSidebarTaskStatus(gtx, statusLabel, taskStatusFromLabel(row.Status))
										})
									}),
								)
							})
						}))
					}
					if menuOpen {
						cardChildren = append(cardChildren, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 6, Bottom: 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return s.layoutSessionMenuPopover(gtx, row)
							})
						}))
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, cardChildren...)
				},
			)
		})
	})
	if isFocused {
		widget.Border{Color: s.theme.primary, CornerRadius: shapeMedium, Width: 1}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: dims.Size}
		})
	}
	leftPad := unit.Dp(sidebarRowIndent)
	if row.Pinned {
		leftPad = unit.Dp(8)
	}
	return desktopInset{Top: 2, Bottom: 2, Left: leftPad, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: dims.Size}
	})
}

func sidebarSessionWidgetKey(sessionID string, agentIDs ...string) string {
	agentID := ""
	if len(agentIDs) > 0 {
		agentID = agentIDs[0]
	}
	return sessionRefStorageKey(desktopstate.SessionRef{AgentID: agentID, SessionID: sessionID})
}

func (s *shell) sessionMenuButton(sessionID string, agentIDs ...string) *widget.Clickable {
	key := sidebarSessionWidgetKey(sessionID, agentIDs...)
	if s.sessionMenuButtons[key] == nil {
		s.sessionMenuButtons[key] = new(widget.Clickable)
	}
	return s.sessionMenuButtons[key]
}

func (s *shell) sessionPinButton(sessionID string, agentIDs ...string) *widget.Clickable {
	key := sidebarSessionWidgetKey(sessionID, agentIDs...)
	if s.sessionPinButtons[key] == nil {
		s.sessionPinButtons[key] = new(widget.Clickable)
	}
	return s.sessionPinButtons[key]
}

func (s *shell) sessionQuickRenameButton(sessionID string, agentIDs ...string) *widget.Clickable {
	key := sidebarSessionWidgetKey(sessionID, agentIDs...)
	if s.sessionQuickRenameButtons[key] == nil {
		s.sessionQuickRenameButtons[key] = new(widget.Clickable)
	}
	return s.sessionQuickRenameButtons[key]
}

func (s *shell) sessionQuickDeleteButton(sessionID string, agentIDs ...string) *widget.Clickable {
	key := sidebarSessionWidgetKey(sessionID, agentIDs...)
	if s.sessionQuickDeleteButtons[key] == nil {
		s.sessionQuickDeleteButtons[key] = new(widget.Clickable)
	}
	return s.sessionQuickDeleteButtons[key]
}

func (s *shell) layoutMiniMenuButton(gtx layout.Context, button *widget.Clickable, label string) layout.Dimensions {
	semantic.Button.Add(gtx.Ops)
	semantic.DescriptionOp("Open conversation actions").Add(gtx.Ops)
	background := color.NRGBA{}
	foreground := s.theme.onSurfaceVariant
	if button.Hovered() {
		background = s.theme.surfaceContainerHighest
		foreground = s.theme.onSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Pt(gtx.Dp(22), gtx.Dp(22))
		gtx.Constraints.Max = image.Pt(gtx.Dp(22), gtx.Dp(22))
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if label == "⋮" {
					return s.layoutActionIcon(gtx, iconKebab, 14, foreground)
				}
				return s.layoutLabel(gtx, label, textLabelMedium, font.Bold, foreground, 1)
			})
		})
	})
	return dims
}

func (s *shell) layoutMiniIconButton(gtx layout.Context, button *widget.Clickable, label string, normalColor color.NRGBA) layout.Dimensions {
	background := color.NRGBA{}
	foreground := normalColor
	if button.Hovered() {
		background = s.theme.surfaceContainerHighest
		foreground = s.theme.onSurface
	}
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Pt(gtx.Dp(22), gtx.Dp(22))
		gtx.Constraints.Max = image.Pt(gtx.Dp(22), gtx.Dp(22))
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if label == "✕" || label == "×" {
					return s.layoutActionIcon(gtx, iconClose, 12, foreground)
				}
				return s.layoutLabel(gtx, label, textLabelSmall, font.Bold, foreground, 1)
			})
		})
	})
	return dims
}

func (s *shell) layoutCountPill(gtx layout.Context, count int) layout.Dimensions {
	return s.roundedSurface(gtx, shapeFull, s.theme.surfaceContainerHigh, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, fmt.Sprintf("%d", count), textLabelSmall, font.Medium, s.theme.onSurfaceVariant, 1)
		})
	})
}

func (s *shell) layoutMiniButton(gtx layout.Context, button *widget.Clickable, label string, fg color.NRGBA) layout.Dimensions {
	dims := button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = gtx.Dp(24)
		background := s.theme.surfaceContainer
		if button.Hovered() {
			background = s.theme.surfaceContainerHighest
		}
		return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
			return desktopInset{Top: 3, Bottom: 3, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.layoutLabel(gtx, label, textLabelSmall, font.Medium, fg, 1)
			})
		})
	})
	return dims
}

func (s *shell) layoutSessionMenuPopover(gtx layout.Context, row sidebarRow) layout.Dimensions {
	pinLabel := "Pin"
	if row.Pinned {
		pinLabel = "Unpin"
	}

	if s.menuPinButton.Clicked(gtx) {
		s.menuSessionID = ""
		s.onTogglePinSession(row.AgentID, row.SessionID)
	}
	if s.menuRenameButton.Clicked(gtx) {
		s.menuSessionID = ""
		s.editingSessionID = row.SessionID
		s.editingSessionAgentID = row.AgentID
		s.sessionRenameEditor.SetText(row.Title)
	}
	if s.menuDeleteButton.Clicked(gtx) {
		s.menuSessionID = ""
		s.deletingSessionID = row.SessionID
		s.deletingSessionAgentID = row.AgentID
		s.deletingSessionTitle = row.Title
	}

	return s.roundedBorderSurface(gtx, shapeSmall, s.theme.surfaceContainerHighest, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 4, Bottom: 4, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceBetween}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMiniButton(gtx, &s.menuPinButton, pinLabel, s.theme.primary)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMiniButton(gtx, &s.menuRenameButton, "Rename", s.theme.onSurface)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return s.layoutMiniButton(gtx, &s.menuDeleteButton, "Delete", s.theme.onErrorContainer)
				}),
			)
		})
	})
}

func (s *shell) layoutDeleteModal(gtx layout.Context) layout.Dimensions {
	if s.deleteModalCancelButton.Clicked(gtx) || s.deleteModalScrim.Clicked(gtx) {
		s.deletingSessionID = ""
		s.deletingSessionTitle = ""
	}

	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{R: 0, G: 0, B: 0, A: 160}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			return s.deleteModalScrim.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			})
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(380)
			gtx.Constraints.Max.X = gtx.Dp(440)
			return s.roundedBorderSurface(gtx, shapeExtraLarge, s.theme.surfaceContainerHigh, s.theme.outlineVariant, 1, func(gtx layout.Context) layout.Dimensions {
				return desktopInset{Top: 20, Bottom: 20, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return s.layoutLabel(gtx, "Delete Session", textTitleMedium, font.Bold, s.theme.onSurface, 1)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return desktopInset{Top: 8, Bottom: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								msg := fmt.Sprintf("Are you sure you want to delete %q? This will delete all conversation turns and cannot be undone.", s.deletingSessionTitle)
								return s.layoutLabel(gtx, msg, textBodyMedium, font.Normal, s.theme.onSurfaceVariant, 3)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle, Spacing: layout.SpaceEnd}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return desktopInset{Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return s.layoutButton(gtx, &s.deleteModalCancelButton, "Cancel", true, func() {
											s.deletingSessionID = ""
											s.deletingSessionTitle = ""
										})
									})
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return s.layoutDangerButton(gtx, &s.deleteModalConfirmButton, "Delete", true, func() {
										id := s.deletingSessionID
										agentID := s.deletingSessionAgentID
										s.deletingSessionID = ""
										s.deletingSessionAgentID = ""
										s.deletingSessionTitle = ""
										if s.onDeleteSession != nil && id != "" {
											s.onDeleteSession(agentID, id)
										}
									})
								}),
							)
						}),
					)
				})
			})
		}),
	)
}

func (s *shell) layoutSidebarTaskStatus(gtx layout.Context, label string, status desktopstate.TaskStatus) layout.Dimensions {
	background, foreground := s.taskStatusColors(status)
	return s.roundedSurface(gtx, shapeSmall, background, func(gtx layout.Context) layout.Dimensions {
		return desktopInset{Top: 1, Bottom: 1, Left: 6, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.layoutLabel(gtx, label, textLabelSmall, font.SemiBold, foreground, 1)
		})
	})
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
		key := sidebarSessionWidgetKey(session.ID, session.AgentID)
		live[key] = struct{}{}
		if s.sessionButtons[key] == nil {
			s.sessionButtons[key] = new(widget.Clickable)
		}
		if s.sessionMenuButtons[key] == nil {
			s.sessionMenuButtons[key] = new(widget.Clickable)
		}
		if s.sessionPinButtons[key] == nil {
			s.sessionPinButtons[key] = new(widget.Clickable)
		}
		if s.sessionQuickRenameButtons[key] == nil {
			s.sessionQuickRenameButtons[key] = new(widget.Clickable)
		}
		if s.sessionQuickDeleteButtons[key] == nil {
			s.sessionQuickDeleteButtons[key] = new(widget.Clickable)
		}
	}
	for sessionID := range s.sessionButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionButtons, sessionID)
		}
	}
	for sessionID := range s.sessionMenuButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionMenuButtons, sessionID)
		}
	}
	for sessionID := range s.sessionPinButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionPinButtons, sessionID)
		}
	}
	for sessionID := range s.sessionQuickRenameButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionQuickRenameButtons, sessionID)
		}
	}
	for sessionID := range s.sessionQuickDeleteButtons {
		if _, ok := live[sessionID]; !ok {
			delete(s.sessionQuickDeleteButtons, sessionID)
		}
	}
	if revision != 0 {
		s.buttonRevision = revision
		s.buttonRevisionSet = true
	}
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
