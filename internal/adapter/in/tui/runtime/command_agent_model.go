package runtime

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"strings"
)

func (m *bubbleModel) handleAgentCommand(argument string) tea.Cmd {
	arg := strings.TrimSpace(argument)
	if arg == "" {
		current := m.agentProfile
		if current == "" {
			current = "universal"
		}
		m.appendLine(fmt.Sprintf("Active agent profile: %s", commandStyle.Render(current)))
		m.appendLine("Available profiles:")
		m.appendLine("  universal - Primary adaptive software engineering orchestrator")
		m.appendLine("  strength - Substantial implementation, fixes, and refactors")
		m.appendLine("  agility - Fast read-only exploration and tracing")
		m.appendLine("  intelligence - Deep reasoning, architecture, and high-risk engineering")
		m.appendLine("Switch profile: /agent <" + agent.ProfileList("|") + ">")
		m.refreshViewport()
		return nil
	}
	prof, err := agent.ParseProfile(arg)
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	m.agentProfile = string(prof)
	m.reconfigureRunner()
	m.appendLine(successStyle.Render(fmt.Sprintf("Agent profile switched to %s.", prof)))
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) executeConversationCommand(name, argument string) tea.Cmd {
	switch name {
	case "transcript":
		m.panes.showTranscript = true
		m.refreshTranscriptViewport(true)
	case "todo":
		switch strings.ToLower(strings.TrimSpace(argument)) {
		case "":
			m.toggleTodoPane()
		case "show":
			if !m.panes.bottom.has(todoInspectViewID) {
				m.panes.bottom.push(&todoPaneView{})
			}
			m.requestRelayout()
		case "hide":
			m.panes.bottom.remove(todoInspectViewID)
			m.requestRelayout()
		default:
			m.appendError("usage: /todo [show|hide]")
		}
	case "clear":
		m.resetTranscript()
		m.refreshViewport()
	case "new":
		m.resetConversation()
		m.refreshViewport()
	}
	return nil
}

func (m *bubbleModel) selectModelDirect(modelID string) tea.Cmd {
	prov := strings.TrimSpace(m.activeProvider)
	if prov == "" {
		prov = model.DefaultOpenCodeName
	}
	return m.beginModelSelect(prov, modelID, !m.modelIDKnown(prov, modelID))
}

func (m *bubbleModel) executeModelCommand(argument string) tea.Cmd {
	arg := strings.TrimSpace(argument)
	switch arg {
	case "add":
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneView())
			m.requestRelayout()
		}
		return nil
	case "free":
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneViewWithPreset(model.DefaultOpenCodeName))
			m.requestRelayout()
		}
		return nil
	case "", "select":
		return m.openModelSelectPane()
	default:
		return m.selectModelDirect(arg)
	}
}
