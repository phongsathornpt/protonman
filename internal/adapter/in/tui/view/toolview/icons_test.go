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
		{"web", tool.KindWeb, tool.NameWeb, tuistyle.UnicodeIcons.Web},
		{"read", tool.KindRead, tool.NameRead, tuistyle.UnicodeIcons.Read},
		{"directory", tool.KindRead, tool.NameLS, tuistyle.UnicodeIcons.Dir},
		{"search", tool.KindGrep, "grep", tuistyle.UnicodeIcons.Search},
		{"git", tool.KindGit, "git", tuistyle.UnicodeIcons.Git},
		{"exec", tool.KindBash, "bash", tuistyle.UnicodeIcons.Exec},
		{"edit", tool.KindEdit, "edit", tuistyle.UnicodeIcons.Edit},
		{"todo", tool.KindTask, tool.NameTodo, tuistyle.UnicodeIcons.TodoActive},
		{"agent", tool.KindAgent, "subagent", tuistyle.UnicodeIcons.Agent},
		{"image", tool.KindRead, "image", tuistyle.UnicodeIcons.Image},
	}
	for _, tt := range tests {
		if got := KindGlyphWithIcons(tuistyle.UnicodeIcons, tt.kind, tt.toolName); got != tt.want {
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
		{"unicode png", tuistyle.UnicodeIcons, "screenshot.png", tuistyle.UnicodeIcons.Image},
		{"unicode quoted jpeg", tuistyle.UnicodeIcons, `"photo.jpeg"`, tuistyle.UnicodeIcons.Image},
		{"unicode webp", tuistyle.UnicodeIcons, "chart.webp", tuistyle.UnicodeIcons.Image},
		{"ascii gif", tuistyle.ASCIIIcons, "anim.gif", tuistyle.ASCIIIcons.Image},
		{"unicode code file", tuistyle.UnicodeIcons, "main.go", tuistyle.UnicodeIcons.Read},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KindGlyphWithTarget(tt.icons, tool.KindRead, tool.NameRead, tt.target); got != tt.want {
				t.Fatalf("KindGlyphWithTarget(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}
