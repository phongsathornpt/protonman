package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/phongsathornpt/proton/internal/app"
)

func (m *bubbleModel) executeSessionCommand(name string) tea.Cmd {
	if name == "session" {
		m.appendLine("session: " + m.sessionID)
		if m.workspaceKey != "" {
			m.appendLine("workspace: " + m.workspaceKey)
		}
		m.appendLine(fmt.Sprintf("messages: %d", len(m.messages)))
		m.refreshViewport()
		return nil
	}
	if m.sessions == nil {
		m.appendError("session store is unavailable")
		m.refreshViewport()
		return nil
	}
	summaries, err := m.sessions.ListSummaries(m.ctx, app.SessionListOptions{WorkspaceKey: m.workspaceKey, Limit: 20})
	if err != nil {
		m.appendError("list sessions: " + err.Error())
		m.refreshViewport()
		return nil
	}
	if len(summaries) == 0 {
		m.appendLine("No resumable sessions for this workspace.")
		m.refreshViewport()
		return nil
	}
	m.appendLine("Recent sessions:")
	for _, summary := range summaries {
		marker := " "
		if summary.ID == m.sessionID {
			marker = "*"
		}
		profile := summary.AgentProfile
		if profile == "" {
			profile = "-"
		}
		preview := summary.Preview
		if preview == "" {
			preview = "(empty session)"
		}
		m.appendLine(fmt.Sprintf("%s %s  %s  %s", marker, summary.ID, profile, truncateWithEllipsis(preview, 72)))
	}
	m.appendLine("Resume with: proton session resume <session-id>")
	m.refreshViewport()
	return nil
}
