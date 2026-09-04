package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
)

const maxSlashRows = 6
const slashViewID = "slash"

type slashCommand struct {
	name        string
	aliases     []string
	description string
	takesArgs   bool
	prefixTag   string
	scope       string
}

var slashCatalog = []slashCommand{
	{name: "help", description: "list commands"},
	{name: "tools", description: "list tools"},
	{name: "skills", description: "list agent skills (or /skills active)", takesArgs: true},
	{name: "skill", description: "show, activate, or toggle an agent skill", takesArgs: true},
	{name: "mode", description: "show or set permission mode", takesArgs: true},
	{name: "ask", description: "switch to ask permission mode"},
	{name: "always-approve", aliases: []string{"yolo"}, description: "allow non-denied calls"},
	{name: "plan", description: "toggle plan flag", takesArgs: true},
	{name: "transcript", aliases: []string{"history"}, description: "open transcript"},
	{name: "todo", description: "show the TODO pane"},
	{name: "clear", description: "clear the visible transcript"},
	{name: "new", description: "start a new conversation"},
	{name: "provider", aliases: []string{"model", "providers"}, description: "configure model providers (e.g. /provider add)", takesArgs: true},
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

type slashContextKind int

const (
	slashKindCommand slashContextKind = iota
	slashKindSkill
)

type slashContext struct {
	kind   slashContextKind
	prefix string
	lead   string
	query  string
}

func (m bubbleModel) parseSlashContext() (slashContext, bool) {
	if m.bottom == nil || m.bottom.bashMode() || m.bottom.has(permissionViewID) || m.bottom.has(skillsViewID) {
		return slashContext{}, false
	}
	prompt := m.bottom.prompt()
	if prompt == nil {
		return slashContext{}, false
	}
	value := prompt.Value()
	if !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, ":") {
		return slashContext{}, false
	}
	prefix := value[:1]
	body := value[1:]

	for _, cmd := range []string{"skill", "skills"} {
		if strings.HasPrefix(body, cmd+" ") {
			rest := strings.TrimPrefix(body, cmd+" ")
			lead := prefix + cmd + " "
			for _, verb := range []string{"toggle", "deactivate", "disable", "remove", "off", "activate", "enable", "on"} {
				if strings.HasPrefix(rest, verb+" ") {
					query := strings.TrimPrefix(rest, verb+" ")
					return slashContext{
						kind:   slashKindSkill,
						prefix: prefix,
						lead:   lead + verb + " ",
						query:  query,
					}, true
				}
				if rest == verb {
					return slashContext{}, false
				}
			}
			return slashContext{
				kind:   slashKindSkill,
				prefix: prefix,
				lead:   lead,
				query:  rest,
			}, true
		}
	}

	if strings.Contains(body, " ") {
		return slashContext{}, false
	}
	return slashContext{
		kind:   slashKindCommand,
		prefix: prefix,
		lead:   prefix,
		query:  body,
	}, true
}

func (m bubbleModel) slashQuery() (prefix string, query string, ok bool) {
	sc, ok := m.parseSlashContext()
	if !ok {
		return "", "", false
	}
	return sc.prefix, sc.query, true
}

func (m bubbleModel) slashMatches() []slashCommand {
	sc, ok := m.parseSlashContext()
	if !ok {
		return nil
	}
	if sc.kind == slashKindSkill {
		if m.skills == nil {
			return nil
		}
		matches := make([]slashCommand, 0)
		for _, s := range m.skills.List() {
			if sc.query == "" || fuzzyContains(s.Name, sc.query) || fuzzyContains(s.Description, sc.query) {
				box := "[ ]"
				if m.skills.IsActivated(s.Name) {
					box = "[x]"
				}
				matches = append(matches, slashCommand{
					name:        s.Name,
					description: s.Description,
					prefixTag:   box,
					scope:       string(s.Scope),
					takesArgs:   false,
				})
			}
		}
		return matches
	}

	matches := make([]slashCommand, 0)
	for _, command := range slashCatalog {
		if command.matches(sc.query) {
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
		view.index = len(matches) - 1
	}
	if view.index >= len(matches) {
		view.index = 0
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
	sc, _ := m.parseSlashContext()

	prompt := m.bottom.prompt()
	var insertion string
	if sc.kind == slashKindSkill {
		insertion = sc.lead + selected.name
	} else {
		insertion = sc.prefix + selected.name
		if selected.takesArgs {
			insertion += " "
			prompt.SetValue(insertion)
			prompt.CursorEnd()
			m.bottom.remove(slashViewID)
			return true, nil
		}
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

func truncateWithEllipsis(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
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
	sc, _ := m.parseSlashContext()
	isSkill := sc.kind == slashKindSkill
	lines := make([]string, 0, len(visible)+1)

	maxName := 16
	if isSkill {
		for _, c := range visible {
			if len(c.name) > maxName {
				maxName = len(c.name)
			}
		}
		if maxName > 26 {
			maxName = 26
		}
	}

	for i, command := range visible {
		selected := offset+i == index
		cursor := "  "
		if selected {
			cursor = glyphPrompt
		}

		var row string
		if isSkill {
			box := command.prefixTag
			if box == "" {
				box = "[ ]"
			}
			nameStr := truncateWithEllipsis(command.name, maxName)
			scopeStr := ""
			if command.scope != "" {
				scopeStr = fmt.Sprintf("[%-7s]", command.scope)
			}
			consumed := 2 + len(box) + 1 + maxName + 1 + 9 + 1
			remaining := maxInt(10, m.width-consumed-2)
			descStr := truncateWithEllipsis(command.description, remaining)
			row = fmt.Sprintf("%s%s %-*s %-9s %s", cursor, box, maxName, nameStr, scopeStr, descStr)
		} else {
			label := "/" + command.name
			remaining := maxInt(10, m.width-20)
			descStr := truncateWithEllipsis(command.description, remaining)
			row = fmt.Sprintf("%s%-16s %s", cursor, label, descStr)
		}

		if selected {
			lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(accentAssistant).Render(row))
		} else {
			lines = append(lines, mutedStyle.Render(row))
		}
	}
	if len(matches) > maxSlashRows {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  (item %d of %d)", index+1, len(matches))))
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
		return m.handleSkillsCommand(false, argument, parts)
	case "skill":
		return m.handleSkillsCommand(true, argument, parts)
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
	case "ask":
		if err := m.service.SetMode(permission.ModeAsk); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		m.setPlanEnabled(false)
		m.appendLine("permission mode: " + permission.ModeAsk.String())
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
	case "clear":
		m.resetTranscript()
		m.refreshViewport()
		return nil
	case "new":
		m.resetConversation()
		m.refreshViewport()
		return nil
	case "provider", "model", "providers":
		if argument == "add" || argument == "" {
			if !m.bottom.has(providerViewID) {
				m.bottom.push(newProviderPaneView())
				m.relayout()
			}
			return nil
		}
		m.appendLine(mutedStyle.Render("Usage: /provider add  (configures AI model provider, e.g. protonman)"))
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

func (m *bubbleModel) handleSkillsCommand(isSkillSingle bool, argument string, parts []string) tea.Cmd {
	trimmedArg := strings.TrimSpace(argument)
	if isSkillSingle && trimmedArg == "" {
		m.appendError("usage: /skill <name> or /skill toggle <name>")
		m.refreshViewport()
		return nil
	}

	if m.skills == nil || len(m.skills.List()) == 0 {
		m.appendLine("No agent skills discovered.")
		m.appendLine("Place skills in ~/.proton/skills/ or .proton/skills/ (with PROTON_TRUST_PROJECT=1).")
		m.refreshViewport()
		return nil
	}

	if trimmedArg == "" {
		skillsList := m.skills.List()
		activeCount := len(m.skills.ActivatedList())
		m.appendLine(fmt.Sprintf("Agent Skills (%d/%d active):", activeCount, len(skillsList)))
		maxPrint := 8
		if len(skillsList) <= maxPrint {
			for _, s := range skillsList {
				box := "[ ]"
				if m.skills.IsActivated(s.Name) {
					box = "[x]"
				}
				cleanDesc := truncateWithEllipsis(s.Description, maxInt(20, m.width-len(s.Name)-20))
				m.appendLine(fmt.Sprintf("  %s %s [%s]: %s", box, s.Name, s.Scope, cleanDesc))
			}
		} else {
			printed := 0
			for _, s := range skillsList {
				if m.skills.IsActivated(s.Name) {
					cleanDesc := truncateWithEllipsis(s.Description, maxInt(20, m.width-len(s.Name)-20))
					m.appendLine(fmt.Sprintf("  [x] %s [%s]: %s", s.Name, s.Scope, cleanDesc))
					printed++
				}
			}
			for _, s := range skillsList {
				if printed >= maxPrint {
					break
				}
				if !m.skills.IsActivated(s.Name) {
					cleanDesc := truncateWithEllipsis(s.Description, maxInt(20, m.width-len(s.Name)-20))
					m.appendLine(fmt.Sprintf("  [ ] %s [%s]: %s", s.Name, s.Scope, cleanDesc))
					printed++
				}
			}
			remaining := len(skillsList) - printed
			if remaining > 0 {
				m.appendLine(fmt.Sprintf("  … and %d more skills. (Browse all in picker below, or use /skill <name>)", remaining))
			}
		}
		m.bottom.push(&skillsPaneView{})
		m.relayout()
		return nil
	}

	if trimmedArg == "active" {
		active := m.skills.ActivatedList()
		if len(active) == 0 {
			m.appendLine("No active agent skills in this session.")
			m.appendLine("Activate skills using /skill <name> or the activate_skill tool.")
		} else {
			m.appendLine(fmt.Sprintf("Active Agent Skills (%d):", len(active)))
			for _, name := range active {
				if s, ok := m.skills.Lookup(name); ok {
					cleanDesc := truncateWithEllipsis(s.Description, maxInt(20, m.width-len(s.Name)-20))
					m.appendLine(fmt.Sprintf("  [x] %s [%s]: %s", s.Name, s.Scope, cleanDesc))
				}
			}
		}
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

	if trimmedArg == "deactivate" || trimmedArg == "disable" || trimmedArg == "remove" || trimmedArg == "off" {
		if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
			m.appendError(fmt.Sprintf("usage: /skill %s <name>", trimmedArg))
			m.refreshViewport()
			return nil
		}
		target := strings.TrimSpace(parts[2])
		if _, ok := m.skills.Lookup(target); !ok {
			m.appendError(fmt.Sprintf("skill %q not found; try /skills to list available skills", target))
			m.refreshViewport()
			return nil
		}
		if !m.skills.IsActivated(target) {
			m.appendLine(fmt.Sprintf("[ ] Skill %q is not active.", target))
			m.refreshViewport()
			return nil
		}
		m.skills.Deactivate(target)
		m.appendLine(fmt.Sprintf("[ ] Skill %q deactivated.", target))
		m.refreshViewport()
		return nil
	}

	target := trimmedArg
	if (trimmedArg == "activate" || trimmedArg == "enable" || trimmedArg == "on") && len(parts) >= 3 {
		target = strings.TrimSpace(parts[2])
	}

	s, ok := m.skills.Lookup(target)
	if !ok {
		m.appendError(fmt.Sprintf("skill %q not found; try /skills to list available skills", target))
		m.refreshViewport()
		return nil
	}
	if m.skills.IsActivated(s.Name) {
		m.appendLine(fmt.Sprintf("[x] Skill %q is already active. Use /skill toggle %s to deactivate.", s.Name, s.Name))
		m.refreshViewport()
		return nil
	}

	m.skills.MarkActivated(s.Name)
	m.appendLine(fmt.Sprintf("[x] Activated skill %s [%s]: %s", s.Name, s.Scope, s.Description))
	if len(s.Resources) > 0 {
		m.appendLine("Bundled resources:")
		for _, r := range s.Resources {
			m.appendLine("  - " + r)
		}
	}
	m.refreshViewport()
	return nil
}
