package cmdpolicy

import (
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

// kindsByName maps every canonical slash-command name to its kind. The name
// list is owned by slashview.Catalog; TestKindTableMatchesCatalog enforces
// exact set equality with it so the two registries cannot drift silently.
var kindsByName = map[string]Kind{
	"help":       KindHelp,
	"permission": KindPermission,
	"low":        KindLow,
	"skills":     KindSkills,
	"goal":       KindGoal,
	"clear":      KindClear,
	"todo":       KindTodo,
	"model":      KindModel,
	"provider":   KindProvider,
	"agents":     KindAgents,
	"call":       KindCall,
	"resume":     KindResume,
	"quit":       KindQuit,
}

func Classify(line string) Command {
	parsed := slashview.ParseCommand(line)
	kind := kindsByName[slashview.CanonicalName(parsed.Name)]

	return Command{
		Kind:     kind,
		Name:     parsed.Name,
		Argument: parsed.Argument,
		Parts:    parsed.Parts,
		Rest:     parsed.Rest,
	}
}
