//go:build desktop

package desktop

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	headerTitleMaxRunes = 72
	headerMetaMaxRunes  = 56
)

func (a *application) renderSessionChrome() {
	a.mu.Lock()
	activeID := a.state.ActiveSessionID
	var active desktopstate.SessionState
	var activeProject desktopstate.ProjectState
	found := false
	for _, session := range a.state.Sessions {
		if session.ID == activeID {
			active = session
			found = true
			break
		}
	}
	activeProject, _ = a.projectByIDLocked(a.state.ActiveProjectID)
	a.mu.Unlock()

	title := "Welcome to Protonman Desktop"
	meta := "Select a session"
	inspector := "_No session context loaded._"
	contextLabel := "Context"
	if !found {
		if activeProject.ID != "" {
			title = activeProject.Name
			meta = "Project overview · " + projectFolderNames(activeProject)
		}
	}
	if found {
		title = sessionDisplayTitle(active)
		meta = sessionDisplayMeta(active)
		if agent := a.agentNameFor(active.AgentID); agent != "" {
			meta = agent + " · " + meta
		}
		inspector = renderInspector(active)
		contextLabel = contextSummaryLabel(active)
	}

	fyne.Do(func() {
		a.sessionTitle.SetText(title)
		a.sessionMeta.SetText(meta)
		a.contextContent.ParseMarkdown(inspector)
		a.contextContent.Refresh()
		a.contextToggle.SetText(contextLabel)
	})
}

func (a *application) agentNameFor(agentID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if profile, ok := a.profiles[strings.TrimSpace(agentID)]; ok {
		return profile.DisplayName
	}
	return ""
}

func sessionDisplayTitle(session desktopstate.SessionState) string {
	if title := strings.TrimSpace(session.Title); title != "" {
		return compactText(title, headerTitleMaxRunes)
	}
	return "Session " + shortID(session.ID)
}

func sessionDisplayMeta(session desktopstate.SessionState) string {
	workspace := strings.TrimSpace(session.WorkspaceName)
	if workspace == "" {
		workspace = "workspace"
	}
	workspace = compactText(workspace, headerMetaMaxRunes)
	if session.Status == desktopstate.TaskIdle {
		return workspace
	}
	return workspace + " · " + string(session.Status)
}

func renderInspector(session desktopstate.SessionState) string {
	var out strings.Builder
	out.WriteString(renderSessionContext(session.Context))
	out.WriteString(renderMemory(session.Context.Memory))
	text := strings.TrimSpace(out.String())
	if text == "" {
		return "_No goal, todo, or memory for this session._"
	}
	return text
}

func contextSummaryLabel(session desktopstate.SessionState) string {
	count := len(session.Context.Todo.Items) + len(session.Context.Memory.Workspace) + len(session.Context.Memory.Global)
	if strings.TrimSpace(session.Context.Goal) != "" {
		count++
	}
	if count == 0 {
		return "Context"
	}
	return fmt.Sprintf("Context %d", count)
}

func compactText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes == 1 {
		return "…"
	}
	return strings.TrimSpace(string(runes[:maxRunes-1])) + "…"
}
