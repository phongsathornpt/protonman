//go:build desktop

package desktop

import (
	"sort"
	"strings"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type sidebarRowKind uint8

const (
	sidebarProjectRow sidebarRowKind = iota
	sidebarSessionRow
	sidebarWorkspaceRow = sidebarProjectRow
)

type sidebarRow struct {
	Kind          sidebarRowKind
	ProjectID     string
	ProjectName   string
	WorkspaceKey  string
	WorkspaceName string
	SessionID     string
	SessionCount  int
}

func (a *application) rebuildSidebarRowsLocked() {
	projects := filterProjects(a.state.Projects, a.state.Sessions, a.sidebarQuery)
	rows := make([]sidebarRow, 0, len(projects))
	for _, project := range projects {
		rows = append(rows, sidebarRow{Kind: sidebarProjectRow, ProjectID: project.ID, ProjectName: project.Name})
	}
	a.sidebarRows = rows
}

func filterProjects(projects []desktopstate.ProjectState, sessions []desktopstate.SessionState, query string) []desktopstate.ProjectState {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]desktopstate.ProjectState(nil), projects...)
	}
	matched := make(map[string]bool)
	for _, session := range sessions {
		if strings.Contains(strings.ToLower(strings.Join([]string{session.Title, session.WorkspaceName, session.Workspace, session.ID}, "\n")), query) {
			matched[session.ProjectID] = true
		}
	}
	filtered := make([]desktopstate.ProjectState, 0, len(projects))
	for _, project := range projects {
		haystack := strings.ToLower(project.Name)
		for _, folder := range project.Folders {
			haystack += "\n" + strings.ToLower(folder.Path)
		}
		if strings.Contains(haystack, query) || matched[project.ID] {
			filtered = append(filtered, project)
		}
	}
	return filtered
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

func sidebarRowIndexForProject(rows []sidebarRow, projectID string) int {
	for i := range rows {
		if rows[i].Kind == sidebarProjectRow && rows[i].ProjectID == projectID {
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
