package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func parseSubagentsEnabled(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "enable", "enabled":
		return true, nil
	case "off", "false", "disable", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("subagents must be on or off")
	}
}

func subagentsEnabledLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func (m *bubbleModel) handleSubagentsCommand(argument string) tea.Cmd {
	arg := strings.TrimSpace(argument)
	if arg == "" {
		m.appendLine("Subagents: " + commandStyle.Render(subagentsEnabledLabel(m.subagentsEnabled)))
		if !m.subagentsEnabled && len(m.agents.List()) > 0 {
			m.appendMuted("New delegation is disabled; existing agents remain manageable.")
		}
		m.refreshViewport()
		return nil
	}
	enabled, err := parseSubagentsEnabled(arg)
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
