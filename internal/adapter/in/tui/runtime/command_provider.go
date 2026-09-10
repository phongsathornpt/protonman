package runtime

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
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
		if !m.panes.bottom.has(providerViewID) {
			if preset != "" {
				m.pushProviderPane(newProviderPaneViewWithPreset(preset))
			} else {
				m.pushProviderPane(newProviderPaneView())
			}
			m.requestRelayout()
		}
		return nil
	}
	if subCmd == "" || subCmd == "select" {
		if !m.panes.bottom.has(providerSelectViewID) {
			m.panes.bottom.push(newProviderSelectPaneView(m))
			m.requestRelayout()
		}
		return nil
	}
	if subCmd == "list" {
		m.appendProviderList()
		return nil
	}
	for name := range m.providers {
		if strings.EqualFold(name, subCmd) {
			return m.beginProviderSelect(name)
		}
	}
	if p := model.LookupPreset(subCmd); p != nil {
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneViewWithPreset(p.ID))
			m.requestRelayout()
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
