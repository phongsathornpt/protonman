package history

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestReasoningCellLifecycle(t *testing.T) {
	started := time.Unix(100, 0)
	cell := &ReasoningCell{
		StartedAt: started,
		Duration:  3200 * time.Millisecond,
		Content:   "First let's analyze the problem.\nThen plan the solution.",
		Streaming: true,
		Expanded:  true,
		Spinner:   "⠋",
		Icons:     tuistyle.UnicodeIcons,
	}

	// 1. Streaming state renders Thinking indicator and content
	rendered := strings.Join(cell.RenderWidth(80), "\n")
	plain := ansi.Strip(rendered)
	if !strings.Contains(plain, "Thinking") || !strings.Contains(plain, "3.2s") {
		t.Fatalf("expected streaming thinking header, got:\n%s", plain)
	}
	if !strings.Contains(plain, "analyze the problem") {
		t.Fatalf("expected streaming thinking content, got:\n%s", plain)
	}

	// 2. Mark completed and collapsed
	cell.Streaming = false
	cell.SetExpanded(false)
	cell.TokenCount = 450
	collapsed := strings.Join(cell.RenderWidth(80), "\n")
	plainCollapsed := ansi.Strip(collapsed)
	if !strings.Contains(plainCollapsed, "Thought for 3.2s · 450 tokens") {
		t.Fatalf("expected collapsed summary line, got:\n%s", plainCollapsed)
	}
	if strings.Contains(plainCollapsed, "analyze the problem") {
		t.Fatalf("collapsed cell should not display thought body, got:\n%s", plainCollapsed)
	}

	// 3. Toggle expanded
	cell.ToggleExpanded()
	if !cell.IsExpanded() {
		t.Fatal("expected cell to be expanded after toggle")
	}
	expanded := strings.Join(cell.RenderWidth(80), "\n")
	plainExpanded := ansi.Strip(expanded)
	if !strings.Contains(plainExpanded, "Thought for 3.2s · 450 tokens") || !strings.Contains(plainExpanded, "analyze the problem") {
		t.Fatalf("expected expanded thought body after toggle, got:\n%s", plainExpanded)
	}
}

func TestPatchCellSmartFoldingAndToggle(t *testing.T) {
	// Large diff (> 8 lines) smart-folds by default
	largeDiff := strings.Join([]string{
		"@@ -1,5 +1,15 @@",
		"+ line 1",
		"+ line 2",
		"+ line 3",
		"+ line 4",
		"+ line 5",
		"+ line 6",
		"+ line 7",
		"+ line 8",
		"+ line 9",
		"+ line 10",
	}, "\n")

	cell := &PatchCell{
		Name:      "edit",
		Paths:     []string{"large.go"},
		Additions: 10,
		Deletions: 0,
		Diff:      largeDiff,
		Icons:     tuistyle.UnicodeIcons,
	}

	// Default smart fold for 10 additions should be collapsed
	if cell.IsExpanded() {
		t.Fatal("expected 10 additions diff to be collapsed by default")
	}
	collapsed := strings.Join(cell.RenderWidth(80), "\n")
	plainCollapsed := ansi.Strip(collapsed)
	if !strings.Contains(plainCollapsed, "diff (10 lines)") {
		t.Fatalf("expected fold indicator in collapsed large diff, got:\n%s", plainCollapsed)
	}

	// Explicitly toggle expanded
	cell.ToggleExpanded()
	if !cell.IsExpanded() {
		t.Fatal("expected cell to be expanded after toggle")
	}
	expanded := strings.Join(cell.RenderWidth(80), "\n")
	plainExpanded := ansi.Strip(expanded)
	if !strings.Contains(plainExpanded, "+ line 10") {
		t.Fatalf("expected all diff lines when expanded, got:\n%s", plainExpanded)
	}
}

func TestExecCellSmartFoldingAndToggle(t *testing.T) {
	longOutput := strings.Repeat("log entry\n", 20)
	cell := &ExecCell{
		Command:  "make test",
		Stdout:   longOutput,
		Icons:    tuistyle.UnicodeIcons,
		Duration: 500 * time.Millisecond,
	}

	// Long output (> 8 lines) should smart-fold
	if cell.IsExpanded() {
		t.Fatal("expected long exec output to be collapsed by default")
	}
	collapsed := strings.Join(cell.RenderWidth(80), "\n")
	plainCollapsed := ansi.Strip(collapsed)
	if !strings.Contains(plainCollapsed, "17 more") {
		t.Fatalf("expected folded output in collapsed exec cell, got:\n%s", plainCollapsed)
	}

	// Toggle expand
	cell.ToggleExpanded()
	if !cell.IsExpanded() {
		t.Fatal("expected cell to be expanded after toggle")
	}
	expanded := strings.Join(cell.RenderWidth(80), "\n")
	if strings.Count(expanded, "log entry") < 15 {
		t.Fatalf("expected full output lines when expanded, got:\n%s", expanded)
	}
}

func TestAgentRunCellStepsAndFolding(t *testing.T) {
	cell := &AgentRunCell{
		AgentID: "worker-1",
		Profile: agent.ProfileStrength,
		Task:    "implement feature",
		State:   agent.StateRunning,
		Summary: "implementing changes",
		Icons:   tuistyle.UnicodeIcons,
	}

	cell.AppendStep(AgentStepRecord{ToolName: "read", Target: "main.go", Summary: "reading main.go"})
	cell.AppendStep(AgentStepRecord{ToolName: "edit", Target: "main.go", Summary: "updated main.go (+10 -2)"})

	// Default running is collapsed
	if cell.IsExpanded() {
		t.Fatal("expected running agent to be collapsed by default")
	}

	// Toggle expanded
	cell.ToggleExpanded()
	if !cell.IsExpanded() {
		t.Fatal("expected agent cell to be expanded after toggle")
	}
	expanded := strings.Join(cell.RenderWidth(80), "\n")
	plainExpanded := ansi.Strip(expanded)
	if !strings.Contains(plainExpanded, "reading main.go") || !strings.Contains(plainExpanded, "updated main.go (+10 -2)") {
		t.Fatalf("expected nested steps timeline in expanded agent cell, got:\n%s", plainExpanded)
	}
}

func TestHistoryStateCollapsibleNavigation(t *testing.T) {
	state := NewHistoryState(100)
	reasoning := &ReasoningCell{Content: "some thoughts", Expanded: false}
	patch := &PatchCell{Name: "edit", Diff: "@@ -1 +1 @@\n+change", Additions: 1}
	state.Append(reasoning)
	state.Append(patch)

	collapsibles := state.CollapsibleCells()
	if len(collapsibles) != 2 {
		t.Fatalf("expected 2 collapsible cells, got %d", len(collapsibles))
	}

	// Highlighting
	state.SetHighlightedCell(reasoning)
	if !reasoning.Highlighted {
		t.Fatal("expected reasoning cell to be highlighted")
	}
	if patch.Highlighted {
		t.Fatal("expected patch cell not to be highlighted")
	}

	state.SetHighlightedCell(patch)
	if reasoning.Highlighted {
		t.Fatal("expected reasoning cell to clear highlight")
	}
	if !patch.Highlighted {
		t.Fatal("expected patch cell to be highlighted")
	}

	// Line lookup
	lines := state.RenderLines()
	if len(lines) == 0 {
		t.Fatal("expected rendered lines")
	}
	found := state.CollapsibleCellAtLine(0)
	if found == nil {
		t.Fatal("expected collapsible cell at line 0")
	}
}
