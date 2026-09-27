package question

import (
	"strings"
	"testing"

	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
)

func TestQuestionViewRendering(t *testing.T) {
	snap := QuestionSnapshot{
		Width:       80,
		Height:      24,
		Question:    "Which database should we use for user settings?",
		Options:     []string{"SQLite", "PostgreSQL"},
		Multiple:    false,
		Index:       0,
		Tone:        panecommon.ToneUser,
		WriteInMode: false,
	}
	res := QuestionView(snap)
	if len(res.Rows) == 0 {
		t.Fatal("expected non-empty rows")
	}
	joined := strings.Join(res.Rows, "\n")
	if !strings.Contains(joined, "Question from Assistant") {
		t.Fatal("expected title row")
	}
	if !strings.Contains(joined, "SQLite") || !strings.Contains(joined, "PostgreSQL") {
		t.Fatal("expected option rows")
	}
	if !strings.Contains(joined, "Write custom response") {
		t.Fatal("expected write-in option")
	}
}

func TestQuestionViewWriteInMode(t *testing.T) {
	snap := QuestionSnapshot{
		Width:       80,
		Height:      24,
		Question:    "Any additional requirements?",
		Options:     nil,
		WriteInMode: true,
		WriteInText: "Add auth tokens",
	}
	res := QuestionView(snap)
	joined := strings.Join(res.Rows, "\n")
	if !strings.Contains(joined, "Add auth tokens") {
		t.Fatal("expected typed text in write-in mode")
	}
}

func TestQuestionViewMultiQuestionProgress(t *testing.T) {
	snap := QuestionSnapshot{
		Width:          80,
		Height:         24,
		Question:       "Which cache layer?",
		Options:        []string{"Redis", "In-memory"},
		QuestionIndex:  1,
		TotalQuestions: 3,
		Recommended:    "Redis",
	}
	res := QuestionView(snap)
	joined := strings.Join(res.Rows, "\n")
	if !strings.Contains(joined, "Question 2 of 3") {
		t.Fatalf("expected progress header, got:\n%s", joined)
	}
	if !strings.Contains(joined, "Redis (Recommended)") {
		t.Fatalf("expected recommended badge, got:\n%s", joined)
	}
	if !strings.Contains(joined, "esc/←: back") {
		t.Fatalf("expected back option in help footer, got:\n%s", joined)
	}
}
