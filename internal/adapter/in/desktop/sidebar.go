//go:build desktop

package desktop

import (
	"sort"
	"strings"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type sidebarRowKind uint8

const (
	sidebarWorkspaceRow sidebarRowKind = iota
	sidebarSessionRow
)

type sidebarRow struct {
	Kind          sidebarRowKind
	WorkspaceKey  string
	WorkspaceName string
	SessionID     string
	SessionCount  int
}

func (a *application) rebuildSidebarRowsLocked() {
	a.sidebarRows = buildSidebarRowsWithCollapsed(filterSessions(a.state.Sessions, a.sidebarQuery), a.collapsedWorkspaces)
}

func filterSessions(sessions []desktopstate.SessionState, query string) []desktopstate.SessionState {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]desktopstate.SessionState(nil), sessions...)
	}

	filtered := make([]desktopstate.SessionState, 0, len(sessions))
	for _, session := range sessions {
		haystack := strings.ToLower(strings.Join([]string{
			session.Title,
			session.WorkspaceName,
			session.Workspace,
			session.WorkspaceKey,
			session.ID,
		}, "\n"))
		if strings.Contains(haystack, query) {
			filtered = append(filtered, session)
		}
	}
	return filtered
}

func buildSidebarRows(sessions []desktopstate.SessionState) []sidebarRow {
	return buildSidebarRowsWithCollapsed(sessions, nil)
}

func buildSidebarRowsWithCollapsed(sessions []desktopstate.SessionState, collapsed map[string]bool) []sidebarRow {
	sorted := append([]desktopstate.SessionState(nil), sessions...)
	sort.SliceStable(sorted, func(i, j int) bool {
		leftWorkspace := strings.ToLower(strings.TrimSpace(sorted[i].WorkspaceName))
		rightWorkspace := strings.ToLower(strings.TrimSpace(sorted[j].WorkspaceName))
		if leftWorkspace != rightWorkspace {
			return leftWorkspace < rightWorkspace
		}
		return sorted[i].ID > sorted[j].ID
	})

	groups := desktopstate.GroupSessionsByWorkspace(sorted)
	rows := make([]sidebarRow, 0, len(sorted)+len(groups))
	for _, group := range groups {
		rows = append(rows, sidebarRow{
			Kind:          sidebarWorkspaceRow,
			WorkspaceKey:  group.Key,
			WorkspaceName: group.Name,
			SessionCount:  len(group.Sessions),
		})
		if collapsed[group.Key] {
			continue
		}
		for _, session := range group.Sessions {
			rows = append(rows, sidebarRow{
				Kind:          sidebarSessionRow,
				WorkspaceKey:  group.Key,
				WorkspaceName: group.Name,
				SessionID:     session.ID,
			})
		}
	}
	return rows
}

func sidebarRowIndexForSession(rows []sidebarRow, sessionID string) int {
	for i := range rows {
		if rows[i].Kind == sidebarSessionRow && rows[i].SessionID == sessionID {
			return i
		}
	}
	return -1
}

func inferredWorkspaceName(title, sessionID string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	suffix := " · " + strings.TrimSpace(sessionID)
	if strings.HasSuffix(title, suffix) {
		return strings.TrimSpace(strings.TrimSuffix(title, suffix))
	}
	return ""
}
