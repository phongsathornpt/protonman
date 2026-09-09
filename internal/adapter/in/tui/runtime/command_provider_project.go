package runtime

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"strings"
)

func (m *bubbleModel) executeProviderCommand(line, rawName string) tea.Cmd {
	cmdLine := strings.TrimSpace(strings.TrimPrefix(line, "/"))
	cmdLine = strings.TrimSpace(strings.TrimPrefix(cmdLine, rawName))
	fields := strings.Fields(cmdLine)
	subCmd, preset := "", ""
	if len(fields) > 0 {
		subCmd = fields[0]
	}
	if len(fields) > 1 {
		preset = fields[1]
	}
	if subCmd == "add" {
		if !m.bottom.has(providerViewID) {
			if preset != "" {
				m.bottom.push(newProviderPaneViewWithPreset(preset))
			} else {
				m.bottom.push(newProviderPaneView())
			}
			m.relayout()
		}
		return nil
	}
	if subCmd == "" || subCmd == "select" {
		if !m.bottom.has(providerSelectViewID) {
			m.bottom.push(newProviderSelectPaneView(m))
			m.relayout()
		}
		return nil
	}
	if subCmd == "list" {
		m.appendProviderList()
		return nil
	}
	for name := range m.providers {
		if strings.EqualFold(name, subCmd) {
			return saveActiveProviderCmd(name)
		}
	}
	if p := model.LookupPreset(subCmd); p != nil {
		if !m.bottom.has(providerViewID) {
			m.bottom.push(newProviderPaneViewWithPreset(p.ID))
			m.relayout()
		}
		return nil
	}
	m.appendLine(mutedStyle.Render(fmt.Sprintf("unknown provider %q; try /provider, /provider list, or /provider add", subCmd)))
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) appendProviderList() {
	if len(m.providers) == 0 {
		m.appendLine(mutedStyle.Render("No providers configured yet. Use /provider to see supported providers."))
	} else {
		m.appendLine(brandStyle.Render("Configured Providers:"))
		for name, p := range m.providers {
			activeTag := ""
			if strings.EqualFold(name, m.activeProvider) {
				activeTag = " " + successStyle.Render("[active]")
			}
			m.appendLine(fmt.Sprintf("  • %s: %s%s", name, p.BaseURL, activeTag))
		}
		if m.activeModel != "" {
			m.appendLine(mutedStyle.Render(fmt.Sprintf("Active model: %s", m.activeModel)))
		}
	}
	m.refreshViewport()
}

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
	case "permission":
		permArgs := ""
		if len(fields) > 1 {
			permArgs = strings.Join(fields[1:], " ")
		}
		return m.handleProjectPermission(permArgs)
	default:
		m.appendError("usage: /project [status|reload|init|set <setting> <value>|permission <allow|deny|ask> <tool> [pattern]]")
		m.refreshViewport()
		return nil
	}
}
