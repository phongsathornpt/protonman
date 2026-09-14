package toolview

import (
	"testing"

	tuiicon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/icon"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestKindGlyphWithSetUsesProvidedProfile(t *testing.T) {
	tests := []struct {
		name     string
		kind     tool.Kind
		toolName string
		want     string
	}{
		{"web", tool.KindWeb, tool.NameWeb, tuiicon.Nerd.Web},
		{"read", tool.KindRead, tool.NameRead, tuiicon.Nerd.Read},
		{"directory", tool.KindRead, tool.NameLS, tuiicon.Nerd.Dir},
		{"search", tool.KindGrep, "grep", tuiicon.Nerd.Search},
		{"git", tool.KindGit, "git", tuiicon.Nerd.Git},
		{"exec", tool.KindBash, "bash", tuiicon.Nerd.Exec},
		{"edit", tool.KindEdit, "edit", tuiicon.Nerd.Edit},
		{"todo", tool.KindTask, tool.NameTodo, tuiicon.Nerd.TodoActive},
		{"agent", tool.KindAgent, "subagent", tuiicon.Nerd.Agent},
	}
	for _, tt := range tests {
		if got := KindGlyphWithSet(tuiicon.Nerd, tt.kind, tt.toolName); got != tt.want {
			t.Fatalf("%s glyph = %q, want %q", tt.name, got, tt.want)
		}
	}
}
