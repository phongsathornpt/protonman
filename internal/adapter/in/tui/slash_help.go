package tui

import (
	"fmt"
	"strings"
)

func (m *bubbleModel) appendHelp() {
	for _, command := range slashCatalog {
		alias := ""
		if len(command.Aliases) > 0 {
			alias = " (" + strings.Join(prefixNames(command.Aliases), ", ") + ")"
		}
		m.appendLine(fmt.Sprintf("/%-16s %s%s", command.Name, command.Description, alias))
	}
}

func prefixNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, "/"+name)
	}
	return out
}
