package runtime

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

func (m *bubbleModel) executeCommand(line string) tea.Cmd {
	defer m.reconcileLayout()

	parsed := parseCommand(line)
	name := parsed.Name
	argument := parsed.Argument
	parts := parsed.Parts
	switch name {
	case "help":
		m.appendHelp()
	case "permission":
		m.openPermissionModePane()
	case "skills":
		return m.handleSkillsCommand(argument, parts)
	case "goal":
		return m.executeConversationCommand(name, parsed.Rest)
	case "clear", "transcript", "todo":
		return m.executeConversationCommand(name, argument)
	case "model":
		return m.executeModelCommand(argument)
	case "provider":
		return m.executeProviderCommand(line, parsed.Name)
	case "agents":
		return m.openAgentsPane()
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

func (m *bubbleModel) appendHelp() {
	for _, command := range slashCatalog {
		m.appendLine("/" + textview.PadRight(command.Name, 16) + " " + command.Description)
	}
}
