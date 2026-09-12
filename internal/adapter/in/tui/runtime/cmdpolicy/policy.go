package cmdpolicy

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
)

type Kind uint8

const (
	KindUnknown Kind = iota
	KindHelp
	KindPermission
	KindLow
	KindSkills
	KindGoal
	KindClear
	KindTodo
	KindModel
	KindProvider
	KindAgents
	KindCall
	KindResume
	KindQuit
)

type Command struct {
	Kind     Kind
	Name     string
	Argument string
	Parts    []string
	Rest     string
}

func (c Command) IsKnown() bool {
	return c.Kind != KindUnknown
}

func Classify(line string) Command {
	parsed := slashview.ParseCommand(line)
	name := strings.ToLower(strings.TrimSpace(parsed.Name))

	var kind Kind
	switch name {
	case "help":
		kind = KindHelp
	case "permission":
		kind = KindPermission
	case "low":
		kind = KindLow
	case "skills":
		kind = KindSkills
	case "goal":
		kind = KindGoal
	case "clear":
		kind = KindClear
	case "todo":
		kind = KindTodo
	case "model":
		kind = KindModel
	case "provider":
		kind = KindProvider
	case "agents":
		kind = KindAgents
	case "call":
		kind = KindCall
	case "resume":
		kind = KindResume
	case "quit":
		kind = KindQuit
	default:
		kind = KindUnknown
	}

	return Command{
		Kind:     kind,
		Name:     parsed.Name,
		Argument: parsed.Argument,
		Parts:    parsed.Parts,
		Rest:     parsed.Rest,
	}
}

func Usage(kind Kind) string {
	switch kind {
	case KindClear:
		return "usage: /clear"
	case KindTodo:
		return "usage: /todo [show|hide]"
	case KindCall:
		return "usage: /call <tool> <json>"
	case KindResume:
		return "usage: /resume [session-id|latest]"
	case KindSkills:
		return "usage: /skills [toggle|activate|deactivate|check|lock] <name>"
	default:
		return ""
	}
}
