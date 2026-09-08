package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/phongsathornpt/proton/internal/core/permission"
)

func (m *bubbleModel) executePermissionCommand(name, argument string) tea.Cmd {
	switch name {
	case "mode":
		if argument == "" {
			m.appendLine("permission mode: " + m.service.Mode().String())
			m.refreshViewport()
			return nil
		}
		mode, err := permission.ParseMode(argument)
		if err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		if err := m.setPermissionMode(mode); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		if mode == permission.ModeAlwaysApprove {
			m.setPlanEnabled(false)
		}
		m.appendLine("permission mode: " + mode.String())
	case "ask":
		if err := m.setPermissionMode(permission.ModeAsk); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		m.setPlanEnabled(false)
		m.appendLine("permission mode: " + permission.ModeAsk.String())
	case "always-approve":
		if err := m.setPermissionMode(permission.ModeAlwaysApprove); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		m.setPlanEnabled(false)
		m.appendLine("permission mode: " + permission.ModeAlwaysApprove.String())
	case "plan":
		m.setPlanMode(argument)
	}
	m.refreshViewport()
	return nil
}
