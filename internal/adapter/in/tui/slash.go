package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/phongsathornpt/proton/internal/core/tool"
	"github.com/phongsathornpt/proton/internal/feature/agent"
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
	{name: "skills", aliases: []string{"skill"}, description: "browse, activate, or toggle agent skills (/skills [name|active|toggle])", takesArgs: true},
	{name: "project", aliases: []string{"proton"}, description: "inspect or edit project-local Proton settings (/project [status|init|set ...])", takesArgs: true},
	{name: "config", description: "edit user-level Proton settings (/config set subagents <on|off>)", takesArgs: true},
	{name: "session", description: "show the active session"},
	{name: "sessions", description: "list resumable sessions for this workspace"},
	{name: "agents", description: "inspect live and retained subagents"},
	{name: "subagents", description: "show or toggle subagent delegation (/subagents [on|off])", takesArgs: true},
	{name: "agent", aliases: []string{"profile"}, description: "show or set agent profile (/agent [" + agent.ProfileList("|") + "])", takesArgs: true},
	{name: "reasoning", aliases: []string{"thinking"}, description: "show or set session reasoning effort (/reasoning [auto|none|low|medium|high|xhigh|max])", takesArgs: true},
	{name: "mode", description: "show or set permission mode", takesArgs: true},
	{name: "ask", description: "switch to ask permission mode"},
	{name: "always-approve", aliases: []string{"yolo"}, description: "allow non-denied calls"},
	{name: "plan", description: "toggle plan flag", takesArgs: true},
	{name: "transcript", aliases: []string{"history"}, description: "open transcript"},
	{name: "todo", description: "show the TODO pane"},
	{name: "clear", description: "clear the visible transcript"},
	{name: "new", description: "start a new conversation"},
	{name: "model", aliases: []string{"models"}, description: "select active model (/model, /model <id>, /model free, /model add)", takesArgs: true},
	{name: "provider", aliases: []string{"providers"}, description: "select or configure model providers (/provider, /provider <name>, /provider add, /provider list)", takesArgs: true},
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

func canonicalSlashName(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	for _, command := range slashCatalog {
		if clean == command.name {
			return command.name
		}
		for _, alias := range command.aliases {
			if clean == alias {
				return command.name
			}
		}
	}
	return clean
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
	return tool.TruncateRunes(s, maxLen)
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
