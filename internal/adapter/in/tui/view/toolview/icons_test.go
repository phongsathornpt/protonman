package toolview

import (
	"testing"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestKindGlyphWithIconsUsesProvidedProfile(t *testing.T) {
	tests := []struct {
		name     string
		kind     tool.Kind
		toolName string
		want     string
	}{
		{"web", tool.KindWeb, tool.NameWeb, tuistyle.NerdIcons.Web},
		{"read", tool.KindRead, tool.NameRead, tuistyle.NerdIcons.Read},
		{"directory", tool.KindRead, tool.NameLS, tuistyle.NerdIcons.Dir},
		{"search", tool.KindGrep, "grep", tuistyle.NerdIcons.Search},
		{"git", tool.KindGit, "git", tuistyle.NerdIcons.Git},
		{"exec", tool.KindBash, "bash", tuistyle.NerdIcons.Exec},
		{"edit", tool.KindEdit, "edit", tuistyle.NerdIcons.Edit},
		{"todo", tool.KindTask, tool.NameTodo, tuistyle.NerdIcons.TodoActive},
		{"agent", tool.KindAgent, "subagent", tuistyle.NerdIcons.Agent},
	}
	for _, tt := range tests {
		if got := KindGlyphWithIcons(tuistyle.NerdIcons, tt.kind, tt.toolName); got != tt.want {
			t.Fatalf("%s glyph = %q, want %q", tt.name, got, tt.want)
		}
	}
}
