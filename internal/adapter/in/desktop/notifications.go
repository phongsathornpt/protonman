//go:build desktop

package desktop

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func (a *application) notifyPermission(item desktopstate.PermissionRequest) {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = "Permission required"
	}
	session := a.sessionNotificationLabel(item.SessionID)
	content := title
	if session != "" {
		content = session + " · " + title
	}
	a.sendNativeNotification("Protonman needs permission", content)
}

func (a *application) notifyTurnFinished(sessionID string, turnErr error) {
	a.mu.Lock()
	active := a.state.ActiveSessionID == sessionID
	label := sessionNotificationLabelLocked(a.state, sessionID)
	a.mu.Unlock()
	if turnErr == nil && active {
		return
	}
	if label == "" {
		label = "Session " + shortID(sessionID)
	}
	if turnErr != nil {
		a.sendNativeNotification("Protonman task failed", label+" · "+compactNotificationText(turnErr.Error()))
		return
	}
	a.sendNativeNotification("Protonman task completed", label+" finished")
}

func (a *application) sessionNotificationLabel(sessionID string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return sessionNotificationLabelLocked(a.state, sessionID)
}

func sessionNotificationLabelLocked(state desktopstate.State, sessionID string) string {
	for _, session := range state.Sessions {
		if session.ID != sessionID {
			continue
		}
		if title := strings.TrimSpace(session.Title); title != "" {
			return title
		}
		if name := strings.TrimSpace(session.WorkspaceName); name != "" {
			return name + " · " + shortID(session.ID)
		}
		return "Session " + shortID(session.ID)
	}
	return ""
}

func compactNotificationText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	const max = 160
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return fmt.Sprintf("%s…", string(runes[:max-1]))
}

func (a *application) sendNativeNotification(title, content string) {
	if a.desktopApp == nil {
		return
	}
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || content == "" {
		return
	}
	a.desktopApp.SendNotification(&fyne.Notification{Title: title, Content: content})
}
