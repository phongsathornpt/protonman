package runtime

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

func (m *bubbleModel) executeCommand(line string) tea.Cmd {
	defer m.reconcileLayout()

	rawName, argument, parts := splitCommand(line)
	name := canonicalSlashName(rawName)
	switch name {
	case "help":
		m.appendHelp()
	case "permission":
		m.openPermissionModePane()
	case "skills":
		return m.handleSkillsCommand(argument, parts)
	case "transcript", "todo":
		return m.executeConversationCommand(name, argument)
	case "model":
		return m.executeModelCommand(argument)
	case "provider":
		return m.executeProviderCommand(line, rawName)
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
