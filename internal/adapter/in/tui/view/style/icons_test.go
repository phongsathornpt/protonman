package style

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestParseIconMode(t *testing.T) {
	tests := []struct {
		in   string
		want IconMode
		err  bool
	}{
		{"", IconModeAuto, false},
		{"AUTO", IconModeAuto, false},
		{"nerd", IconModeNerd, false},
		{"unicode", IconModeUnicode, false},
		{"ascii", IconModeASCII, false},
		{"emoji-magic", "", true},
	}
	for _, tt := range tests {
		got, err := ParseIconMode(tt.in)
		if (err != nil) != tt.err {
			t.Fatalf("ParseIconMode(%q) error = %v, want error=%v", tt.in, err, tt.err)
		}
		if got != tt.want {
			t.Fatalf("ParseIconMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveIconsAutoUsesSafeFallbacks(t *testing.T) {
	if got := ResolveIcons(IconModeAuto, true); got != UnicodeIcons {
		t.Fatalf("interactive auto = %#v, want UnicodeIcons", got)
	}
	if got := ResolveIcons(IconModeAuto, false); got != ASCIIIcons {
		t.Fatalf("non-interactive auto = %#v, want ASCIIIcons", got)
	}
}

func TestIconProfilesPreservePrefixWidthContract(t *testing.T) {
	profiles := map[string]IconSet{
		"unicode": UnicodeIcons,
		"ascii":   ASCIIIcons,
		"nerd":    NerdIcons,
	}
	for name, icons := range profiles {
		prefixes := []string{
			icons.Prompt, icons.Mark, icons.Tool, icons.ToolSuccess, icons.ToolError,
			icons.ToolDenied, icons.Web, icons.Read, icons.Dir, icons.Search, icons.Exec,
			icons.Edit, icons.Skill, icons.Agent, icons.Git, icons.Generic,
			icons.TodoPending, icons.TodoActive,
		}
		for _, glyph := range prefixes {
			if got := ansi.StringWidth(glyph); got != 2 {
				t.Fatalf("%s prefix %q width = %d, want 2", name, glyph, got)
			}
		}
		if got := ansi.StringWidth(icons.Brand); got != 1 {
			t.Fatalf("%s brand %q width = %d, want 1", name, icons.Brand, got)
		}
	}
}
