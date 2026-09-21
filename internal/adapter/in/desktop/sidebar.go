//go:build desktop

package desktop

import (
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
