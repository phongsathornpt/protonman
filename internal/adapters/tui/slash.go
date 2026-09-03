package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/projectTHORN/proton/internal/domain/model"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

const maxSlashRows = 6
const slashViewID = "slash"

type slashCommand struct {
	name        string
	aliases     []string
	description string
	takesArgs   bool
}

var slashCatalog = []slashCommand{
	{name: "help", description: "list commands"},
	{name: "tools", description: "list tools"},
	{name: "skills", description: "list agent skills (or /skills active)", takesArgs: true},
	{name: "skill", description: "show, activate, or toggle an agent skill", takesArgs: true},
	{name: "mode", description: "show or set permission mode", takesArgs: true},
	{name: "always-approve", aliases: []string{"yolo"}, description: "allow non-denied calls"},
	{name: "plan", description: "toggle plan flag", takesArgs: true},
	{name: "transcript", aliases: []string{"history"}, description: "open transcript"},
	{name: "todo", description: "show the TODO pane"},
	{name: "clear", aliases: []string{"new"}, description: "clear the transcript"},
	{name: "call", description: "run a registered tool", takesArgs: true},
	{name: "quit", aliases: []string{"exit"}, description: "leave Proton"},
}

type slashPaneView struct{ index int }

func (*slashPaneView) ID() string             { return slashViewID }
func (*slashPaneView) ReplacesComposer() bool { return false }
func (v *slashPaneView) Render(m *bubbleModel) string {
	return m.renderSlash(v.index)
}
func (v *slashPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	switch message.String() {
	case "up":
		m.moveSlash(-1)
		return true, nil
	case "down":
		m.moveSlash(1)
		return true, nil
	case "tab":
		_, command := m.acceptSlash(false)
		return true, command
	case "enter":
		_, command := m.acceptSlash(true)
		return true, command
	case "esc":
		// Dismiss completion without destroying the draft. Editing the command
		// token can reopen completion through syncSlashView.
		m.bottom.remove(slashViewID)
		return true, nil
	default:
		return false, nil
	}
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
	if m.bottom == nil || m.bottom.bashMode() || m.bottom.has(permissionViewID) {
		return "", "", false
	}
	prompt := m.bottom.prompt()
	if prompt == nil {
		return "", "", false
	}
	value := prompt.Value()
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

func (m *bubbleModel) slashState() *slashPaneView {
	if m.bottom == nil {
		return nil
	}
	view, _ := m.bottom.find(slashViewID).(*slashPaneView)
	return view
}

func (m *bubbleModel) syncSlashView() {
	if m.bottom == nil {
		return
	}
	matches := m.slashMatches()
	if len(matches) == 0 {
		m.bottom.remove(slashViewID)
		return
	}
	view := m.slashState()
	if view == nil {
		view = &slashPaneView{}
		m.bottom.push(view)
	}
	if view.index >= len(matches) {
		view.index = len(matches) - 1
	}
	if view.index < 0 {
		view.index = 0
	}
}

func (m bubbleModel) slashOpen() bool {
	return len(m.slashMatches()) > 0
}

func (m *bubbleModel) clampSlashIndex() {
	m.syncSlashView()
	view := m.slashState()
	if view == nil {
		return
	}
	matches := m.slashMatches()
	if view.index >= len(matches) {
		view.index = len(matches) - 1
	}
	if view.index < 0 {
		view.index = 0
	}
}

func (m *bubbleModel) moveSlash(delta int) {
	m.syncSlashView()
	view := m.slashState()
	matches := m.slashMatches()
	if view == nil || len(matches) == 0 {
		return
	}
	view.index += delta
	if view.index < 0 {
		view.index = 0
	}
	if view.index >= len(matches) {
		view.index = len(matches) - 1
	}
}

func (m *bubbleModel) acceptSlash(run bool) (applied bool, command tea.Cmd) {
	m.syncSlashView()
	view := m.slashState()
	matches := m.slashMatches()
	if view == nil || len(matches) == 0 {
		return false, nil
	}
	m.clampSlashIndex()
	selected := matches[view.index]
	prefix, _, _ := m.slashQuery()
	insertion := prefix + selected.name
	prompt := m.bottom.prompt()
	if selected.takesArgs {
		insertion += " "
		prompt.SetValue(insertion)
		prompt.CursorEnd()
		m.bottom.remove(slashViewID)
		return true, nil
	}
	if !run {
		prompt.SetValue(insertion)
		prompt.CursorEnd()
		m.bottom.remove(slashViewID)
		return true, nil
	}
	prompt.Reset()
	m.bottom.remove(slashViewID)
	return true, m.dispatch(insertion)
}

func (m bubbleModel) renderSlash(index int) string {
	matches := m.slashMatches()
	if len(matches) == 0 {
		return ""
	}
	if index < 0 {
		index = 0
	}
	if index >= len(matches) {
		index = len(matches) - 1
	}
	visible := matches
	offset := 0
	if len(visible) > maxSlashRows {
		if index >= maxSlashRows {
			offset = index - maxSlashRows + 1
		}
		visible = matches[offset : offset+maxSlashRows]
	}
	lines := make([]string, 0, len(visible))
	for i, command := range visible {
		selected := offset+i == index
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

func (m bubbleModel) slashView() string {
	if view := m.slashState(); view != nil {
		return view.Render(&m)
	}
	return ""
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
	case "skills":
		if m.skills == nil || len(m.skills.List()) == 0 {
			m.appendLine("No agent skills discovered.")
			m.appendLine("Place skills in ~/.proton/skills/ or .proton/skills/ (with PROTON_TRUST_PROJECT=1).")
			m.refreshViewport()
			return nil
		}

		if strings.TrimSpace(argument) == "active" {
			active := m.skills.ActivatedList()
			if len(active) == 0 {
				m.appendLine("No active agent skills in this session.")
				m.appendLine("Activate skills using /skill <name> or the activate_skill tool.")
			} else {
				m.appendLine(fmt.Sprintf("Active Agent Skills (%d):", len(active)))
				for _, name := range active {
					if s, ok := m.skills.Lookup(name); ok {
						m.appendLine(fmt.Sprintf("  [x] %s [%s]: %s", s.Name, s.Scope, s.Description))
					}
				}
			}
			m.refreshViewport()
			return nil
		}

		skillsList := m.skills.List()
		activeCount := len(m.skills.ActivatedList())
		m.appendLine(fmt.Sprintf("Agent Skills (%d/%d active):", activeCount, len(skillsList)))
		for _, s := range skillsList {
			box := "[ ]"
			if m.skills.IsActivated(s.Name) {
				box = "[x]"
			}
			m.appendLine(fmt.Sprintf("  %s %s [%s]: %s", box, s.Name, s.Scope, s.Description))
		}
	case "skill":
		trimmedArg := strings.TrimSpace(argument)
		if trimmedArg == "" {
			m.appendError("usage: /skill <name> or /skill toggle <name>")
			m.refreshViewport()
			return nil
		}
		if m.skills == nil {
			m.appendError("no skills registered")
			m.refreshViewport()
			return nil
		}

		if trimmedArg == "toggle" {
			if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
				m.appendError("usage: /skill toggle <name>")
				m.refreshViewport()
				return nil
			}
			target := strings.TrimSpace(parts[2])
			active, err := m.skills.Toggle(target)
			if err != nil {
				m.appendError(err.Error())
				m.refreshViewport()
				return nil
			}
			state := "deactivated"
			box := "[ ]"
			if active {
				state = "activated"
				box = "[x]"
			}
			m.appendLine(fmt.Sprintf("%s Skill %q %s.", box, target, state))
			m.refreshViewport()
			return nil
		}

		skillName := trimmedArg
		s, ok := m.skills.Lookup(skillName)
		if !ok {
			m.appendError(fmt.Sprintf("skill %q not found; try /skills to list available skills", skillName))
			m.refreshViewport()
			return nil
		}
		m.skills.MarkActivated(s.Name)
		m.appendLine(fmt.Sprintf("[x] Activated skill %s [%s]:", s.Name, s.Scope))
		m.appendLine(s.Instructions)
		if len(s.Resources) > 0 {
			m.appendLine("Bundled resources:")
			for _, r := range s.Resources {
				m.appendLine("  - " + r)
			}
		}
		m.messages = append(m.messages, model.Message{
			Role:    model.RoleUser,
			Content: fmt.Sprintf("Activated skill %s [%s]:\n%s", s.Name, s.Scope, s.Instructions),
		})
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
			m.setPlanEnabled(false)
		}
		m.appendLine("permission mode: " + mode.String())
	case "always-approve", "yolo":
		if err := m.service.SetMode(permission.ModeAlwaysApprove); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		m.setPlanEnabled(false)
		m.appendLine("permission mode: " + permission.ModeAlwaysApprove.String())
	case "plan":
		m.setPlanMode(argument)
	case "transcript", "history":
		m.showTranscript = true
		m.refreshTranscriptViewport(true)
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
