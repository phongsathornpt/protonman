package permissionpolicy

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

type Option int

const (
	AllowOnce Option = iota
	AllowSession
	AllowProject
	AllowGlobal
	Deny
)

type Item struct {
	Option   Option
	Label    string
	Shortcut string
}

func Options(request permission.Request, projectTrusted, hasWorkDir bool) []Item {
	items := []Item{{Option: AllowOnce, Label: "Allow once", Shortcut: "y"}}
	if permission.SessionGrantEligible(request) {
		items = append(items, Item{Option: AllowSession, Label: "Allow for this request this session", Shortcut: "s"})
	}
	if permission.PersistentRuleEligible(request) {
		if projectTrusted && hasWorkDir {
			items = append(items, Item{Option: AllowProject, Label: "Allow and save to project (.protonman/config.toml)", Shortcut: "p"})
		}
		items = append(items, Item{Option: AllowGlobal, Label: "Allow and save globally (~/.protonman/config.toml)", Shortcut: "g"})
	}
	return append(items, Item{Option: Deny, Label: "Deny", Shortcut: "n"})
}

func ShortcutHint(items []Item) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		switch item.Option {
		case AllowOnce:
			parts = append(parts, "y once")
		case AllowSession:
			parts = append(parts, "s session")
		case AllowProject:
			parts = append(parts, "p project")
		case AllowGlobal:
			parts = append(parts, "g global")
		case Deny:
			parts = append(parts, "n deny")
		}
	}
	return strings.Join(parts, " · ")
}
