//go:build desktop

package desktop

import (
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

func buildSidebarRows(sessions []desktopstate.SessionState) []sidebarRow {
	groups := desktopstate.GroupSessionsByWorkspace(sessions)
	rows := make([]sidebarRow, 0, len(sessions)+len(groups))
	for _, group := range groups {
		rows = append(rows, sidebarRow{
			Kind:          sidebarWorkspaceRow,
			WorkspaceKey:  group.Key,
			WorkspaceName: group.Name,
			SessionCount:  len(group.Sessions),
		})
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
