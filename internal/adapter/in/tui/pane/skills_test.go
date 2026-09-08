package pane

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSkillsRowsEmptyState(t *testing.T) {
	rows := SkillsRows(SkillsSnapshot{Width: 80, Height: 24, UserSkillsDisplay: "~/.protonman/skills"})
	plain := ansi.Strip(strings.Join(rows, "\n"))
	if !strings.Contains(plain, "No agent skills discovered") || !strings.Contains(plain, "~/.protonman/skills") {
		t.Fatalf("unexpected empty skills view: %q", plain)
	}
}

func TestSkillsRowsShowsActiveAndSelection(t *testing.T) {
	rows := SkillsRows(SkillsSnapshot{
		Width:  80,
		Height: 24,
		Index:  1,
		Items: []SkillItem{
			{Name: "pdf", Active: true},
			{Name: "slides"},
		},
	})
	plain := ansi.Strip(strings.Join(rows, "\n"))
	for _, want := range []string{"1/2 active", "[x] pdf", "slides", "item 2 of 2"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("skills view missing %q: %q", want, plain)
		}
	}
}

func TestSkillsRowsUsesCompactFooterOnTinyTerminal(t *testing.T) {
	rows := SkillsRows(SkillsSnapshot{Width: 30, Height: 10, Items: []SkillItem{{Name: "pdf"}}})
	plain := ansi.Strip(strings.Join(rows, "\n"))
	if !strings.Contains(plain, "↑/↓ · space · esc") {
		t.Fatalf("tiny footer missing: %q", plain)
	}
	if strings.Contains(plain, "pgup/pgdn") {
		t.Fatalf("tiny footer retained wide controls: %q", plain)
	}
}
