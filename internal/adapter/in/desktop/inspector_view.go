//go:build desktop

package desktop

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func (a *application) renderSessionChrome() {
	a.mu.Lock()
	activeID := a.state.ActiveSessionID
	var active desktopstate.SessionState
	found := false
	for _, session := range a.state.Sessions {
		if session.ID == activeID {
			active = session
			found = true
			break
		}
	}
	a.mu.Unlock()

	title := "protonMAN"
	meta := "Select a session"
	inspector := "_No session context loaded._"
	contextLabel := "Context"
	if found {
		title = sessionDisplayTitle(active)
		meta = sessionDisplayMeta(active)
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

func sessionDisplayTitle(session desktopstate.SessionState) string {
	if title := strings.TrimSpace(session.Title); title != "" {
		return title
	}
	return "Session " + shortID(session.ID)
}

func sessionDisplayMeta(session desktopstate.SessionState) string {
	workspace := strings.TrimSpace(session.WorkspaceName)
	if workspace == "" {
		workspace = "workspace"
	}
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
