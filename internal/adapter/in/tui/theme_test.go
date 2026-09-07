package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCoreGlyphsHaveStableSingleCellWidth(t *testing.T) {
	glyphs := map[string]string{
		"prompt":       glyphPrompt,
		"mark":         glyphMark,
		"success":      glyphToolSuccess,
		"error":        glyphToolError,
		"denied":       glyphToolDenied,
		"web":          glyphWeb,
		"read":         glyphRead,
		"dir":          glyphDir,
		"search":       glyphSearch,
		"exec":         glyphExec,
		"edit":         glyphEdit,
		"skill":        glyphSkill,
		"agent":        glyphAgent,
		"generic":      glyphGeneric,
		"todo_pending": glyphTodoPending,
		"todo_active":  glyphTodoActive,
	}
	for name, glyph := range glyphs {
		if got := ansi.StringWidth(glyph); got != 2 {
			t.Fatalf("%s glyph %q width = %d, want 2 including trailing space", name, glyph, got)
		}
	}
}
