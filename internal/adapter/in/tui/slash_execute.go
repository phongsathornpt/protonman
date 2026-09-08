package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *bubbleModel) executeCommand(line string) tea.Cmd {
	rawName, argument, parts := splitCommand(line)
	name := canonicalSlashName(rawName)
	switch name {
	case "help":
		m.appendHelp()
	case "tools":
		m.appendRegisteredTools()
	case "skills":
		return m.handleSkillsCommand(argument, parts)
	case "project":
		return m.executeProjectCommand(line, rawName)
	case "config":
		return m.executeUserConfigCommand(line, rawName)
	case "session", "sessions":
		return m.executeSessionCommand(name)
	case "mode", "ask", "always-approve", "plan":
		return m.executePermissionCommand(name, argument)
	case "transcript", "todo", "clear", "new":
		return m.executeConversationCommand(name, argument)
	case "model":
		return m.executeModelCommand(argument)
	case "provider":
		return m.executeProviderCommand(line, rawName)
	case "agents":
		return m.openAgentsPane()
	case "subagents":
		return m.handleSubagentsCommand(argument)
	case "agent":
		return m.handleAgentCommand(argument)
	case "reasoning":
		return m.handleReasoningCommand(argument)
	case "call":
		return m.startCall(parts)
	case "quit":
		return tea.Quit
	default:
		m.appendError(fmt.Sprintf("unknown command %q; try /help", name))
	}
	m.refreshViewport()
	return nil
}
