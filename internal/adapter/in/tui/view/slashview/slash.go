package slashview

import (
	"strings"
)

type Command struct {
	Name        string
	Description string
	TakesArgs   bool
	PrefixTag   string
	Scope       string
}

func Catalog() []Command {
	return []Command{
		{Name: "help", Description: "list commands"},
		{Name: "model", Description: "open model setup or select active model (/model [id|free|add])", TakesArgs: true},
		{Name: "provider", Description: "select or configure model providers (/provider [name|add|list])", TakesArgs: true},
		{Name: "skills", Description: "browse, activate, or toggle agent skills (/skills [name|active|toggle])", TakesArgs: true},
		{Name: "agents", Description: "inspect live and retained subagents"},
		{Name: "todo", Description: "show the TODO pane", TakesArgs: true},
		{Name: "transcript", Description: "open or clear transcript (/transcript [clear])", TakesArgs: true},
		{Name: "call", Description: "run a registered tool", TakesArgs: true},
		{Name: "quit", Description: "leave Protonman"},
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

func CanonicalName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
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
	for _, cmd := range []string{"skills"} {
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
