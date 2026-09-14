package icon

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		input string
		want  Mode
	}{
		{"", ModeAuto},
		{"AUTO", ModeAuto},
		{" nerd ", ModeNerd},
		{"unicode", ModeUnicode},
		{"ascii", ModeASCII},
	}
	for _, tt := range tests {
		got, err := ParseMode(tt.input)
		if err != nil {
			t.Fatalf("ParseMode(%q): %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("ParseMode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
	if _, err := ParseMode("emoji-magic"); err == nil {
		t.Fatal("ParseMode accepted unsupported mode")
	}
}

func TestResolveAutoUsesSafeProfiles(t *testing.T) {
	if got := Resolve(ModeAuto, true); got != Unicode {
		t.Fatalf("interactive auto = %#v, want Unicode", got)
	}
	if got := Resolve(ModeAuto, false); got != ASCII {
		t.Fatalf("non-interactive auto = %#v, want ASCII", got)
	}
	if got := Resolve(ModeNerd, false); got != Nerd {
		t.Fatalf("explicit nerd = %#v, want Nerd", got)
	}
}

func TestProfilesPreservePrefixCellContract(t *testing.T) {
	profiles := map[string]Set{
		"unicode": Unicode,
		"ascii":   ASCII,
		"nerd":    Nerd,
	}
	for profile, icons := range profiles {
		glyphs := map[string]string{
			"prompt": icons.Prompt, "mark": icons.Mark, "tool": icons.Tool,
			"success": icons.ToolSuccess, "error": icons.ToolError, "denied": icons.ToolDenied,
			"web": icons.Web, "read": icons.Read, "dir": icons.Dir, "search": icons.Search,
			"exec": icons.Exec, "edit": icons.Edit, "skill": icons.Skill, "agent": icons.Agent,
			"git": icons.Git, "generic": icons.Generic, "todo_pending": icons.TodoPending,
			"todo_active": icons.TodoActive,
		}
		for name, glyph := range glyphs {
			if got := ansi.StringWidth(glyph); got != 2 {
				t.Fatalf("%s %s glyph %q width = %d, want 2", profile, name, glyph, got)
			}
		}
		if got := ansi.StringWidth(icons.Brand); got != 1 {
			t.Fatalf("%s brand %q width = %d, want 1", profile, icons.Brand, got)
		}
	}
}
