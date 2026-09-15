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
		{"image", tool.KindRead, "image", tuistyle.NerdIcons.Image},
	}
	for _, tt := range tests {
		if got := KindGlyphWithIcons(tuistyle.NerdIcons, tt.kind, tt.toolName); got != tt.want {
			t.Fatalf("%s glyph = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestKindGlyphWithTargetUsesImageIconForImages(t *testing.T) {
	tests := []struct {
		name   string
		icons  tuistyle.IconSet
		target string
		want   string
	}{
		{"nerd png", tuistyle.NerdIcons, "screenshot.png", tuistyle.NerdIcons.Image},
		{"nerd quoted jpeg", tuistyle.NerdIcons, `"photo.jpeg"`, tuistyle.NerdIcons.Image},
		{"unicode webp", tuistyle.UnicodeIcons, "chart.webp", tuistyle.UnicodeIcons.Image},
		{"ascii gif", tuistyle.ASCIIIcons, "anim.gif", tuistyle.ASCIIIcons.Image},
		{"nerd code file", tuistyle.NerdIcons, "main.go", tuistyle.NerdIcons.Read},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KindGlyphWithTarget(tt.icons, tool.KindRead, tool.NameRead, tt.target); got != tt.want {
				t.Fatalf("KindGlyphWithTarget(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}
