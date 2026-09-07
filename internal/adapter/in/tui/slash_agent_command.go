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
			current = "default"
		}
		m.appendLine(fmt.Sprintf("Active agent profile: %s", commandStyle.Render(current)))
		m.appendLine("Available profiles:")
		m.appendLine("  pow - Fast implementation and concrete execution")
		m.appendLine("  int - Read-only investigation, tracing, research, and review")
		m.appendLine("  dex - Defensive engineering for complex or high-risk work")
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
