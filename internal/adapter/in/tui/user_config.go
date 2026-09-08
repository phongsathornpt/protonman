package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/adapter/out/config"
	"github.com/projectTHORN/proton/internal/app"
)

type userSettingSavedMsg struct {
	field string
	value any
	err   error
}

func (m *bubbleModel) executeUserConfigCommand(line, rawName string) tea.Cmd {
	cmdLine := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	cmdLine = strings.TrimSpace(strings.TrimPrefix(cmdLine, rawName))
	fields := strings.Fields(cmdLine)
	if len(fields) != 3 || strings.ToLower(fields[0]) != "set" || strings.ToLower(fields[1]) != "subagents" {
		m.appendError("usage: /config set subagents <on|off>")
		m.refreshViewport()
		return nil
	}
	enabled, err := parseSubagentsEnabled(fields[2])
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	return func() tea.Msg {
		err := (app.UserSettings{}).SaveSubagentsEnabled(enabled)
		return userSettingSavedMsg{field: config.FieldAgentSubagentsEnabled, value: enabled, err: err}
	}
}

func (m *bubbleModel) updateUserSettingSaved(message userSettingSavedMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.appendError("failed to save user setting: " + message.err.Error())
		m.refreshViewport()
		return m, nil
	}
	if message.field != config.FieldAgentSubagentsEnabled {
		m.appendError("unsupported user setting")
		m.refreshViewport()
		return m, nil
	}
	enabled := message.value.(bool)
	if m.projectSource(config.FieldAgentSubagentsEnabled) == config.SourceProject {
		m.appendLine(successStyle.Render("User subagent default saved."))
		m.appendMuted("The trusted project override remains effective in this workspace.")
		m.refreshViewport()
		return m, nil
	}
	m.subagentsEnabled = enabled
	m.agents.SetEnabled(enabled)
	if m.projectConfigProvenance == nil {
		m.projectConfigProvenance = make(map[string]config.ValueSource)
	}
	m.projectConfigProvenance[config.FieldAgentSubagentsEnabled] = config.SourceUser
	m.reconfigureRunner()
	m.appendLine(successStyle.Render("User subagent default saved and applied."))
	m.refreshViewport()
	return m, nil
}
