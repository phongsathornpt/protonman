package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/commandutil"
)

func (m *bubbleModel) handleSubagentsCommand(argument string) tea.Cmd {
	arg := strings.TrimSpace(argument)
	if arg == "" {
		m.appendLine("Subagents: " + commandStyle.Render(commandutil.SubagentsEnabledLabel(m.subagentsEnabled)))
		if !m.subagentsEnabled && len(m.agents.List()) > 0 {
			m.appendMuted("New delegation is disabled; existing agents remain manageable.")
		}
		m.refreshViewport()
		return nil
	}
	enabled, err := commandutil.ParseSubagentsEnabled(arg)
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	m.subagentsEnabled = enabled
	m.agents.SetEnabled(enabled)
	m.reconfigureRunner()
	if enabled {
		m.appendLine(successStyle.Render("Subagents enabled."))
	} else {
		m.appendLine(successStyle.Render("Subagents disabled."))
		if len(m.agents.List()) > 0 {
			m.appendMuted("Running and retained agents remain available for lifecycle control.")
		} else {
			m.appendMuted("Universal will handle work directly.")
		}
	}
	m.refreshViewport()
	return nil
}
