package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/feature/agent"
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
