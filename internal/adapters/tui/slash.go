package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

const maxSlashRows = 6

type slashCommand struct {
	name        string
	aliases     []string
	description string
	takesArgs   bool
}

var slashCatalog = []slashCommand{
	{name: "help", description: "list commands"},
	{name: "tools", description: "list tools"},
	{name: "mode", description: "show or set permission mode", takesArgs: true},
	{name: "always-approve", aliases: []string{"yolo"}, description: "allow non-denied calls"},
	{name: "plan", description: "toggle plan flag", takesArgs: true},
	{name: "todo", description: "show the TODO pane"},
	{name: "clear", aliases: []string{"new"}, description: "clear the transcript"},
	{name: "call", description: "run a registered tool", takesArgs: true},
	{name: "quit", aliases: []string{"exit"}, description: "leave Proton"},
}

func isCommandLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, ":")
}

func splitCommand(line string) (name string, argument string, rest []string) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 {
		return "", "", nil
	}
	body := strings.TrimSpace(trimmed[1:])
	parts := strings.SplitN(body, " ", 3)
	if len(parts) == 0 {
		return "", "", nil
	}
	name = strings.TrimSpace(parts[0])
	if len(parts) > 1 {
		argument = strings.TrimSpace(parts[1])
	}
	return name, argument, parts
}

func (c slashCommand) matches(query string) bool {
	if query == "" {
		return true
	}
	if fuzzyContains(c.name, query) {
		return true
	}
	for _, alias := range c.aliases {
		if fuzzyContains(alias, query) {
			return true
		}
	}
	return false
}

func fuzzyContains(target, query string) bool {
	target = strings.ToLower(target)
	query = strings.ToLower(query)
	if strings.Contains(target, query) {
		return true
	}
	ti := 0
	for _, q := range query {
		found := false
		for ti < len(target) {
			if rune(target[ti]) == q {
				ti++
				found = true
				break
			}
			ti++
		}
		if !found {
			return false
		}
	}
	return true
}

func (m bubbleModel) slashQuery() (prefix string, query string, ok bool) {
	if m.bashMode || m.modal != nil {
		return "", "", false
	}
	value := m.prompt.Value()
	if !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, ":") {
		return "", "", false
	}
	prefix = value[:1]
	rest := value[1:]
	if strings.Contains(rest, " ") {
		return prefix, "", false
	}
	return prefix, rest, true
}

func (m bubbleModel) slashMatches() []slashCommand {
	_, query, ok := m.slashQuery()
	if !ok {
		return nil
	}
	matches := make([]slashCommand, 0)
	for _, command := range slashCatalog {
		if command.matches(query) {
			matches = append(matches, command)
		}
	}
	return matches
}

func (m bubbleModel) slashOpen() bool {
	return len(m.slashMatches()) > 0
}

func (m *bubbleModel) clampSlashIndex() {
	matches := m.slashMatches()
	if len(matches) == 0 {
		m.slashIndex = 0
		return
	}
	if m.slashIndex >= len(matches) {
		m.slashIndex = len(matches) - 1
	}
	if m.slashIndex < 0 {
		m.slashIndex = 0
	}
}

func (m *bubbleModel) moveSlash(delta int) {
	matches := m.slashMatches()
	if len(matches) == 0 {
		return
	}
	m.slashIndex += delta
	if m.slashIndex < 0 {
		m.slashIndex = 0
	}
	if m.slashIndex >= len(matches) {
		m.slashIndex = len(matches) - 1
	}
}

func (m *bubbleModel) acceptSlash(run bool) (applied bool, command tea.Cmd) {
	matches := m.slashMatches()
	if len(matches) == 0 {
		return false, nil
	}
	m.clampSlashIndex()
	selected := matches[m.slashIndex]
	prefix, _, _ := m.slashQuery()
	insertion := prefix + selected.name
	if selected.takesArgs {
		insertion += " "
		m.prompt.SetValue(insertion)
		m.prompt.CursorEnd()
		m.slashIndex = 0
		return true, nil
	}
	if !run {
		m.prompt.SetValue(insertion)
		m.prompt.CursorEnd()
		return true, nil
	}
	m.prompt.Reset()
	return true, m.dispatch(insertion)
}

func (m bubbleModel) slashView() string {
	matches := m.slashMatches()
	if len(matches) == 0 {
		return ""
	}
	visible := matches
	offset := 0
	if len(visible) > maxSlashRows {
		if m.slashIndex >= maxSlashRows {
			offset = m.slashIndex - maxSlashRows + 1
		}
		visible = matches[offset : offset+maxSlashRows]
	}
	lines := make([]string, 0, len(visible))
	for i, command := range visible {
		selected := offset+i == m.slashIndex
		label := "/" + command.name
		if selected {
			row := glyphPrompt + fmt.Sprintf("%-16s %s", label, command.description)
			lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(accentAssistant).Render(row))
			continue
		}
		row := fmt.Sprintf("  %-16s %s", label, command.description)
		lines = append(lines, mutedStyle.Render(row))
	}
	return strings.Join(lines, "\n")
}

func (m *bubbleModel) executeCommand(line string) tea.Cmd {
	name, argument, parts := splitCommand(line)
	switch name {
	case "help":
		m.appendHelp()
	case "tools":
		m.appendLine("Registered tools:")
		for _, definition := range m.registry.Definitions() {
			m.appendLine(fmt.Sprintf("- %s [%s]: %s", definition.Name, definition.Kind, definition.Description))
		}
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
		if err := m.service.SetMode(mode); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		if mode == permission.ModeAlwaysApprove {
			m.planMode = false
		}
		m.appendLine("permission mode: " + mode.String())
	case "always-approve", "yolo":
		if err := m.service.SetMode(permission.ModeAlwaysApprove); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		m.planMode = false
		m.appendLine("permission mode: " + permission.ModeAlwaysApprove.String())
	case "plan":
		m.setPlanMode(argument)
	case "todo":
		m.todoHidden = false
		m.appendTodo()
		m.resize(m.width, m.height)
	case "clear", "new":
		m.resetTranscript()
		m.refreshViewport()
		return nil
	case "call":
		return m.startCall(parts)
	case "quit", "exit":
		return tea.Quit
	default:
		m.appendError(fmt.Sprintf("unknown command %q; try /help", name))
	}
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) appendHelp() {
	for _, command := range slashCatalog {
		alias := ""
		if len(command.aliases) > 0 {
			alias = " (" + strings.Join(prefixNames(command.aliases), ", ") + ")"
		}
		m.appendLine(fmt.Sprintf("/%-16s %s%s", command.name, command.description, alias))
	}
}

func prefixNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, "/"+name)
	}
	return out
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
	call, err := tool.NewCall(
		fmt.Sprintf("bubble-%d", m.nextID),
		strings.TrimSpace(parts[1]),
		[]byte(arguments),
	)
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
	call, err := tool.NewCall(
		fmt.Sprintf("bubble-%d", m.nextID),
		"bash",
		[]byte(payload),
	)
	if err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	return m.startTool(call)
}
