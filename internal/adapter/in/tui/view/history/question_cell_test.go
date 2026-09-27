package history

import (
	"strings"
	"testing"
)

func TestQuestionCellRenderWidth(t *testing.T) {
	cell := QuestionCell{
		Question: "Which database?",
		Answer:   "PostgreSQL",
		Declined: false,
	}
	lines := cell.RenderWidth(80)
	if len(lines) == 0 {
		t.Fatal("expected non-empty lines")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Which database?") {
		t.Fatal("expected question text")
	}
	if !strings.Contains(joined, "PostgreSQL") {
		t.Fatal("expected answer text")
	}
	if cell.Kind() != HistoryCellTool {
		t.Fatalf("kind = %v, want HistoryCellTool", cell.Kind())
	}
}

func TestQuestionCellDeclined(t *testing.T) {
	cell := QuestionCell{
		Question: "Confirm deletion?",
		Answer:   "User declined to answer",
		Declined: true,
	}
	lines := cell.RenderWidth(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "[Declined]") {
		t.Fatal("expected declined badge")
	}
	raw := cell.RawLines()
	if len(raw) != 2 || !strings.Contains(raw[1], "[Declined]") {
		t.Fatalf("unexpected raw lines: %+v", raw)
	}
}
