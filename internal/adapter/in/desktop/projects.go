//go:build desktop

package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const projectsPreferencesKey = "projects.v1"

type persistedProject struct {
	ID             string                       `json:"id"`
	Name           string                       `json:"name"`
	Folders        []desktopstate.ProjectFolder `json:"folders"`
	AgentIDs       []string                     `json:"agentIds,omitempty"`
	DefaultAgentID string                       `json:"defaultAgentId,omitempty"`
}

func projectIDForPath(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return "project-" + hex.EncodeToString(sum[:8])
}

func projectNameForPath(path string) string {
	name := filepath.Base(filepath.Clean(path))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "Project"
	}
	return name
}

func loadProjects(preferences preferenceReader) []desktopstate.ProjectState {
	if preferences == nil {
		return nil
	}
	var items []persistedProject
	if raw := strings.TrimSpace(preferences.String(projectsPreferencesKey)); raw != "" {
		if json.Unmarshal([]byte(raw), &items) == nil {
			return normalizeProjects(items)
		}
	}
	return nil
}

func normalizeProjects(items []persistedProject) []desktopstate.ProjectState {
	projects := make([]desktopstate.ProjectState, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		folders := make([]desktopstate.ProjectFolder, 0, len(item.Folders))
		folderSeen := make(map[string]bool)
		for _, folder := range item.Folders {
			path := storedFolderPath(folder.Path)
			if path == "" || folderSeen[path] {
				continue
			}
			folderSeen[path] = true
			folders = append(folders, desktopstate.ProjectFolder{Path: path, Primary: folder.Primary})
		}
		if len(folders) > 0 {
			primary := false
			for i := range folders {
				if folders[i].Primary && !primary {
					primary = true
				} else {
					folders[i].Primary = false
				}
			}
			if !primary {
				folders[0].Primary = true
			}
		}
		name := strings.TrimSpace(item.Name)
		if name == "" && len(folders) > 0 {
			name = projectNameForPath(folders[0].Path)
		}
		if name == "" {
			name = "Project"
		}
		agents := append([]string(nil), item.AgentIDs...)
		sort.Strings(agents)
		defaultAgent := strings.TrimSpace(item.DefaultAgentID)
		if !containsString(agents, defaultAgent) {
			defaultAgent = ""
		}
		projects = append(projects, desktopstate.ProjectState{ID: id, Name: name, Folders: folders, AgentIDs: agents, DefaultAgentID: defaultAgent})
	}
	sort.SliceStable(projects, func(i, j int) bool { return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name) })
	return projects
}

func storedFolderPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return ""
	}
	return filepath.Clean(path)
}

func persistProjects(preferences preferenceWriter, projects []desktopstate.ProjectState) error {
	if preferences == nil {
		return fmt.Errorf("Desktop preferences are unavailable")
	}
	items := make([]persistedProject, 0, len(projects))
	for _, project := range projects {
		items = append(items, persistedProject{ID: project.ID, Name: project.Name, Folders: project.Folders, AgentIDs: project.AgentIDs, DefaultAgentID: project.DefaultAgentID})
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}
	preferences.SetString(projectsPreferencesKey, string(payload))
	return nil
}

func projectForSession(projects []desktopstate.ProjectState, session desktopstate.SessionState) desktopstate.ProjectState {
	for _, project := range projects {
		if project.ID == session.ProjectID {
			return project
		}
	}
	path := validWorkspacePath(session.Workspace)
	if path == "" {
		return desktopstate.ProjectState{}
	}
	return desktopstate.ProjectState{ID: projectIDForPath(path), Name: projectNameForPath(path), Folders: []desktopstate.ProjectFolder{{Path: path, Primary: true}}}
}

func (a *application) ensureProjectForSessionLocked(session *desktopstate.SessionState) {
	if session == nil {
		return
	}
	project := projectForSession(a.state.Projects, *session)
	if project.ID == "" {
		return
	}
	if session.ProjectID == "" {
		session.ProjectID = project.ID
	}
	if session.AgentID != "" && !containsString(project.AgentIDs, session.AgentID) {
		project.AgentIDs = append(project.AgentIDs, session.AgentID)
	}
	if project.DefaultAgentID == "" && session.AgentID != "" {
		project.DefaultAgentID = session.AgentID
	}
	for i := range a.state.Projects {
		if a.state.Projects[i].ID == project.ID && session.AgentID != "" && !containsString(a.state.Projects[i].AgentIDs, session.AgentID) {
			a.state.Projects[i].AgentIDs = append(a.state.Projects[i].AgentIDs, session.AgentID)
			sort.Strings(a.state.Projects[i].AgentIDs)
		}
	}
	for i := range a.state.Projects {
		if a.state.Projects[i].ID == project.ID {
			return
		}
	}
	a.state.Projects = append(a.state.Projects, project)
}

func (a *application) agentForProjectLocked(project desktopstate.ProjectState) string {
	if project.DefaultAgentID != "" {
		if _, ok := a.profiles[project.DefaultAgentID]; ok {
			return project.DefaultAgentID
		}
	}
	for _, agentID := range project.AgentIDs {
		if _, ok := a.profiles[agentID]; ok {
			return agentID
		}
	}
	return a.activeAgentID
}

func (a *application) setActiveProjectAgentLocked(agentID string) {
	if a.state.ActiveProjectID == "" || agentID == "" {
		return
	}
	for i := range a.state.Projects {
		project := &a.state.Projects[i]
		if project.ID != a.state.ActiveProjectID || containsString(project.AgentIDs, agentID) {
			continue
		}
		project.AgentIDs = append(project.AgentIDs, agentID)
		project.DefaultAgentID = agentID
		sort.Strings(project.AgentIDs)
		_ = persistProjects(a.preferences, a.state.Projects)
		return
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func (a *application) migrateProjectsLocked() {
	for i := range a.state.Sessions {
		a.ensureProjectForSessionLocked(&a.state.Sessions[i])
	}
	if a.state.ActiveProjectID == "" && a.state.ActiveSessionID != "" {
		for _, session := range a.state.Sessions {
			if session.ID == a.state.ActiveSessionID {
				a.state.ActiveProjectID = session.ProjectID
				break
			}
		}
	}
}

func (a *application) projectByIDLocked(id string) (desktopstate.ProjectState, bool) {
	for _, project := range a.state.Projects {
		if project.ID == id {
			return project, true
		}
	}
	return desktopstate.ProjectState{}, false
}

func primaryProjectFolder(project desktopstate.ProjectState) string {
	for _, folder := range project.Folders {
		if folder.Primary && validWorkspacePath(folder.Path) != "" {
			return validWorkspacePath(folder.Path)
		}
	}
	if len(project.Folders) > 0 {
		return validWorkspacePath(project.Folders[0].Path)
	}
	return ""
}

func projectAdditionalFolders(project desktopstate.ProjectState, primary string) []string {
	folders := make([]string, 0, len(project.Folders))
	for _, folder := range project.Folders {
		path := validWorkspacePath(folder.Path)
		if path != "" && path != primary {
			folders = append(folders, path)
		}
	}
	return folders
}

func addProjectFolder(project *desktopstate.ProjectState, path string) bool {
	path = validWorkspacePath(path)
	if project == nil || path == "" {
		return false
	}
	for _, folder := range project.Folders {
		if folder.Path == path {
			return false
		}
	}
	project.Folders = append(project.Folders, desktopstate.ProjectFolder{Path: path, Primary: len(project.Folders) == 0})
	return true
}

func (a *application) createProjectFromCurrentFolder() {
	path, err := os.Getwd()
	if err != nil {
		a.setStatus("Create project failed · determine folder: " + err.Error())
		return
	}
	path = validWorkspacePath(path)
	if path == "" {
		a.setStatus("Create project failed · current folder is unavailable")
		return
	}
	a.mu.Lock()
	project := desktopstate.ProjectState{ID: projectIDForPath(path), Name: projectNameForPath(path), Folders: []desktopstate.ProjectFolder{{Path: path, Primary: true}}, AgentIDs: []string{a.activeAgentID}}
	found := false
	for i := range a.state.Projects {
		if a.state.Projects[i].ID == project.ID {
			found = true
			project = a.state.Projects[i]
			break
		}
	}
	if !found {
		a.state.Projects = append(a.state.Projects, project)
	}
	a.state.ActiveProjectID = project.ID
	a.rebuildSidebarRowsLocked()
	err = persistProjects(a.preferences, a.state.Projects)
	a.mu.Unlock()
	if err != nil {
		a.setStatus("Create project failed · " + err.Error())
		return
	}
	a.list.Refresh()
	a.refreshSidebarEmptyState()
	if a.agentSelect != nil {
		a.agentSelect.SetSelected(a.agentForProject(project))
	}
	a.refreshActiveView()
	a.setStatus("Project selected · " + project.Name)
}

func (a *application) addFolderToActiveProject(path string) {
	a.mu.Lock()
	project, ok := a.projectByIDLocked(a.state.ActiveProjectID)
	if !ok {
		a.mu.Unlock()
		a.setStatus("Select a project before adding a folder")
		return
	}
	if !addProjectFolder(&project, path) {
		a.mu.Unlock()
		a.setStatus("Folder already belongs to this project or is unavailable")
		return
	}
	for i := range a.state.Projects {
		if a.state.Projects[i].ID == project.ID {
			a.state.Projects[i] = project
		}
	}
	err := persistProjects(a.preferences, a.state.Projects)
	a.rebuildSidebarRowsLocked()
	a.mu.Unlock()
	if err != nil {
		a.setStatus("Add folder failed · " + err.Error())
		return
	}
	a.list.Refresh()
	a.refreshActiveView()
	a.setStatus("Folder added · " + filepath.Base(path))
}

func (a *application) agentForProject(project desktopstate.ProjectState) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.agentForProjectLocked(project)
}

func projectFolderNames(project desktopstate.ProjectState) string {
	names := make([]string, 0, len(project.Folders))
	for _, folder := range project.Folders {
		name := filepath.Base(folder.Path)
		if folder.Primary {
			name += " *"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return "No folders"
	}
	return strings.Join(names, ", ")
}
