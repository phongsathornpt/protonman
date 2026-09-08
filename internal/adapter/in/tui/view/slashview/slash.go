package slashview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

type Command struct {
	Name        string
	Aliases     []string
	Description string
	TakesArgs   bool
	PrefixTag   string
	Scope       string
}

func Catalog(agentProfiles string) []Command {
	return []Command{
		{Name: "help", Description: "list commands"},
		{Name: "tools", Description: "list tools"},
		{Name: "skills", Aliases: []string{"skill"}, Description: "browse, activate, or toggle agent skills (/skills [name|active|toggle])", TakesArgs: true},
		{Name: "project", Aliases: []string{"protonman", "proton"}, Description: "inspect or edit project-local Protonman settings (/project [status|init|set ...|permission ...])", TakesArgs: true},
		{Name: "config", Description: "edit user-level Protonman settings (/config set <subagents|thinking|tool-calls> <value>, /config permission <allow|deny|ask> <tool> [pattern])", TakesArgs: true},
		{Name: "session", Description: "show the active session"},
		{Name: "sessions", Description: "list resumable sessions for this workspace"},
		{Name: "agents", Description: "inspect live and retained subagents"},
		{Name: "subagents", Description: "show or toggle subagent delegation (/subagents [on|off])", TakesArgs: true},
		{Name: "agent", Aliases: []string{"profile"}, Description: "show or set agent profile (/agent [" + agentProfiles + "])", TakesArgs: true},
		{Name: "reasoning", Aliases: []string{"thinking"}, Description: "show or set session reasoning effort (/reasoning [auto|none|low|medium|high|xhigh|max])", TakesArgs: true},
		{Name: "mode", Description: "show or set permission mode", TakesArgs: true},
		{Name: "ask", Description: "switch to ask permission mode"},
		{Name: "always-approve", Aliases: []string{"yolo"}, Description: "allow non-denied calls"},
		{Name: "plan", Description: "toggle plan flag", TakesArgs: true},
		{Name: "transcript", Aliases: []string{"history"}, Description: "open transcript"},
		{Name: "todo", Description: "show the TODO pane"},
		{Name: "clear", Description: "clear the visible transcript"},
		{Name: "new", Description: "start a new conversation"},
		{Name: "model", Aliases: []string{"models"}, Description: "select active model (/model, /model <id>, /model free, /model add)", TakesArgs: true},
		{Name: "provider", Aliases: []string{"providers"}, Description: "select or configure model providers (/provider, /provider <name>, /provider add, /provider list)", TakesArgs: true},
		{Name: "call", Description: "run a registered tool", TakesArgs: true},
		{Name: "quit", Aliases: []string{"exit"}, Description: "leave Protonman"},
	}
}

func IsCommandLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, ":")
}

func SplitCommand(line string) (name string, argument string, rest []string) {
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

func CanonicalName(catalog []Command, name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	for _, command := range catalog {
		if clean == command.Name {
			return command.Name
		}
		for _, alias := range command.Aliases {
			if clean == alias {
				return command.Name
			}
		}
	}
	return clean
}

func FuzzyContains(target, query string) bool {
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

func (c Command) Matches(query string) bool {
	if query == "" || FuzzyContains(c.Name, query) {
		return true
	}
	for _, alias := range c.Aliases {
		if FuzzyContains(alias, query) {
			return true
		}
	}
	return false
}

type ContextKind uint8

const (
	ContextCommand ContextKind = iota
	ContextSkill
)

type Context struct {
	Kind   ContextKind
	Prefix string
	Lead   string
	Query  string
}

func ParseContext(value string) (Context, bool) {
	if !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, ":") {
		return Context{}, false
	}
	prefix := value[:1]
	body := value[1:]
	for _, cmd := range []string{"skill", "skills"} {
		if !strings.HasPrefix(body, cmd+" ") {
			continue
		}
		rest := strings.TrimPrefix(body, cmd+" ")
		lead := prefix + cmd + " "
		for _, verb := range []string{"toggle", "deactivate", "disable", "remove", "off", "activate", "enable", "on"} {
			if strings.HasPrefix(rest, verb+" ") {
				return Context{Kind: ContextSkill, Prefix: prefix, Lead: lead + verb + " ", Query: strings.TrimPrefix(rest, verb+" ")}, true
			}
			if rest == verb {
				return Context{}, false
			}
		}
		return Context{Kind: ContextSkill, Prefix: prefix, Lead: lead, Query: rest}, true
	}
	if strings.Contains(body, " ") {
		return Context{}, false
	}
	return Context{Kind: ContextCommand, Prefix: prefix, Lead: prefix, Query: body}, true
}

type Skill struct {
	Name        string
	Description string
	Scope       string
	Active      bool
}

func Matches(context Context, catalog []Command, skills []Skill) []Command {
	if context.Kind == ContextSkill {
		matches := make([]Command, 0, len(skills))
		for _, skill := range skills {
			if context.Query != "" && !FuzzyContains(skill.Name, context.Query) && !FuzzyContains(skill.Description, context.Query) {
				continue
			}
			box := "[ ]"
			if skill.Active {
				box = "[x]"
			}
			matches = append(matches, Command{Name: skill.Name, Description: skill.Description, PrefixTag: box, Scope: skill.Scope})
		}
		return matches
	}
	matches := make([]Command, 0, len(catalog))
	for _, command := range catalog {
		if command.Matches(context.Query) {
			matches = append(matches, command)
		}
	}
	return matches
}

func Render(matches []Command, index, width, maxRows int, kind ContextKind) string {
	if len(matches) == 0 {
		return ""
	}
	if index < 0 {
		index = 0
	}
	if index >= len(matches) {
		index = len(matches) - 1
	}
	if maxRows <= 0 {
		maxRows = 1
	}
	visible := matches
	offset := 0
	if len(visible) > maxRows {
		if index >= maxRows {
			offset = index - maxRows + 1
		}
		visible = matches[offset : offset+maxRows]
	}
	isSkill := kind == ContextSkill
	lines := make([]string, 0, len(visible)+1)
	maxName := 16
	if isSkill {
		for _, command := range visible {
			if len(command.Name) > maxName {
				maxName = len(command.Name)
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
			cursor = tuistyle.GlyphPrompt
		}
		var row string
		if isSkill {
			box := command.PrefixTag
			if box == "" {
				box = "[ ]"
			}
			name := textview.TruncateEllipsis(command.Name, maxName)
			scope := ""
			if command.Scope != "" {
				scope = "[" + textview.PadRight(command.Scope, 7) + "]"
			}
			consumed := 2 + len(box) + 1 + maxName + 1 + 9 + 1
			remaining := max(10, width-consumed-2)
			row = cursor + box + " " + textview.PadRight(name, maxName) + " " + textview.PadRight(scope, 9) + " " + textview.TruncateEllipsis(command.Description, remaining)
		} else {
			label := "/" + command.Name
			remaining := max(10, width-20)
			row = cursor + textview.PadRight(label, 16) + " " + textview.TruncateEllipsis(command.Description, remaining)
		}
		if selected {
			lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(tuistyle.AccentAssistant).Render(row))
		} else {
			lines = append(lines, tuistyle.MutedStyle.Render(row))
		}
	}
	if len(matches) > maxRows {
		lines = append(lines, tuistyle.MutedStyle.Render(fmt.Sprintf("  (item %d of %d)", index+1, len(matches))))
	}
	return strings.Join(lines, "\n")
}
