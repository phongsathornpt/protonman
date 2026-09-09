package runtime

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func (m *bubbleModel) appendRegisteredTools() {
	m.appendLine("Registered tools:")
	for _, definition := range m.registry.Definitions() {
		m.appendLine(fmt.Sprintf("- %s [%s]: %s", definition.Name, definition.Kind, definition.Description))
	}
}

func (m *bubbleModel) startCall(parts []string) tea.Cmd {
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		m.appendError("usage: /call <tool> <json>")
		m.refreshViewport()
		return nil
	}
	arguments := "{}"
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		arguments = parts[2]
	}
	m.nextID++
	call, err := tool.NewCall(fmt.Sprintf("bubble-%d", m.nextID), strings.TrimSpace(parts[1]), []byte(arguments))
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	return m.startTool(call)
}

func (m *bubbleModel) startBash(command string) tea.Cmd {
	m.nextID++
	payload := fmt.Sprintf(`{"command":%q}`, command)
	call, err := tool.NewCall(fmt.Sprintf("bubble-%d", m.nextID), "bash", []byte(payload))
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	return m.startTool(call)
}
