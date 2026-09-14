package toolview

import (
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// KindGlyphWithIcons returns the semantic glyph for a tool using the supplied
// terminal icon profile. Existing KindGlyph remains the Unicode compatibility
// path while callers migrate incrementally.
func KindGlyphWithIcons(icons tuistyle.IconSet, kind tool.Kind, name string) string {
	icons = tuistyle.OrUnicodeIcons(icons)
	switch kind {
	case tool.KindWeb:
		return icons.Web
	case tool.KindRead:
		if strings.TrimSpace(name) == tool.NameLS {
			return icons.Dir
		}
		return icons.Read
	case tool.KindGrep:
		return icons.Search
	case tool.KindGit:
		return icons.Git
	case tool.KindBash:
		return icons.Exec
	case tool.KindEdit:
		return icons.Edit
	case tool.KindTask:
		return icons.TodoActive
	case tool.KindAgent:
		return icons.Agent
	}
	if strings.TrimSpace(name) == tool.NameSkill {
		return icons.Skill
	}
	return icons.Generic
}
