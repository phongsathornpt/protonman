package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *bubbleModel) executeProjectCommand(line, rawName string) tea.Cmd {
	cmdLine := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	cmdLine = strings.TrimSpace(strings.TrimPrefix(cmdLine, rawName))
	fields := strings.Fields(cmdLine)
	subCmd := ""
	if len(fields) > 0 {
		subCmd = strings.ToLower(fields[0])
	}
	switch subCmd {
	case "", "status", "reload":
		return m.openProjectPane()
	case "init":
		return m.initProject()
	case "set":
		settingArgs := ""
		if len(fields) > 1 {
			settingArgs = strings.Join(fields[1:], " ")
		}
		return m.handleProjectSet(settingArgs)
	default:
		m.appendError("usage: /project [status|reload|init|set <setting> <value>]")
		m.refreshViewport()
		return nil
	}
}
