package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/agent"
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
		m.appendLine("  pow      - High-velocity, direct execution (action-first, minimal code)")
		m.appendLine("  dex      - Defensive engineering, zero regression (TDD, thorough checks)")
		m.appendLine("  int      - Deep reasoning & systems architect (YAGNI, root cause analysis)")
		m.appendLine("  worker   - General-purpose mutating coding worker")
		m.appendLine("  explorer - Read-only codebase and web search")
		m.appendLine("  reviewer - Code, security, and architecture review")
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
