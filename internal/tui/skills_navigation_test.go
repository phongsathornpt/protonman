package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
)

func newTestSkillsModel(t *testing.T, count int) *bubbleModel {
	t.Helper()
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{
		Name: "read_file",
		Kind: tool.KindRead,
	}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	model.resize(80, 24)

	skills := make([]skill.Skill, 0, count)
	for i := 1; i <= count; i++ {
		skills = append(skills, skill.Skill{
			Name:        fmt.Sprintf("skill-%02d", i),
			Description: fmt.Sprintf("Description for skill %02d", i),
			Scope:       skill.ScopeUser,
		})
	}
	model.skills = skill.NewRegistry(skills...)
	return model
}

func TestSkillsPickerWindowingLargeList(t *testing.T) {
	model := newTestSkillsModel(t, 15)

	// Open skills picker via ctrl+s
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(*bubbleModel)

	if !model.bottom.has(skillsViewID) {
		t.Fatal("expected skills picker to be open")
	}

	render := model.bottom.renderTop(model)
	// Must contain windowing indicators
	if !strings.Contains(render, "item 1 of 15") {
		t.Fatalf("expected 'item 1 of 15' in header, got: %s", render)
	}
	if !strings.Contains(render, "▼ 9 more below") {
		t.Fatalf("expected '▼ 9 more below' indicator, got: %s", render)
	}
	if strings.Contains(render, "▲") {
		t.Fatalf("expected no up arrow at top, got: %s", render)
	}

	// Move down 8 times
	for range 8 {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(*bubbleModel)
	}

	render = model.bottom.renderTop(model)
	if !strings.Contains(render, "item 9 of 15") {
		t.Fatalf("expected 'item 9 of 15', got: %s", render)
	}
	if !strings.Contains(render, "▲") {
		t.Fatalf("expected up arrow when scrolled down, got: %s", render)
	}
	if !strings.Contains(render, "skill-09") {
		t.Fatalf("expected skill-09 to be visible in window, got: %s", render)
	}
}

func TestSkillsPickerWrapAround(t *testing.T) {
	model := newTestSkillsModel(t, 5)

	// Open picker
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(*bubbleModel)

	// At item 1 (index 0), pressing up should wrap to item 5 (index 4)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(*bubbleModel)
	render := model.bottom.renderTop(model)
	if !strings.Contains(render, "item 5 of 5") {
		t.Fatalf("expected wrap-around to item 5 of 5, got: %s", render)
	}

	// At item 5, pressing down should wrap to item 1
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(*bubbleModel)
	render = model.bottom.renderTop(model)
	if !strings.Contains(render, "item 1 of 5") {
		t.Fatalf("expected wrap-around to item 1 of 5, got: %s", render)
	}
}

func TestSkillsPickerFastNavigation(t *testing.T) {
	model := newTestSkillsModel(t, 12)

	// Open picker
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(*bubbleModel)

	// PageDown jumps 5
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(*bubbleModel)
	render := model.bottom.renderTop(model)
	if !strings.Contains(render, "item 6 of 12") {
		t.Fatalf("expected item 6 after pgdown, got: %s", render)
	}

	// End / G jumps to last
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	model = updated.(*bubbleModel)
	render = model.bottom.renderTop(model)
	if !strings.Contains(render, "item 12 of 12") {
		t.Fatalf("expected item 12 after G, got: %s", render)
	}

	// Home / g jumps to first
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	model = updated.(*bubbleModel)
	render = model.bottom.renderTop(model)
	if !strings.Contains(render, "item 1 of 12") {
		t.Fatalf("expected item 1 after g, got: %s", render)
	}

	// Direct number jump '3' jumps to item 3
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	model = updated.(*bubbleModel)
	render = model.bottom.renderTop(model)
	if !strings.Contains(render, "item 3 of 12") {
		t.Fatalf("expected item 3 after number 3, got: %s", render)
	}
}

func TestComposerDraftPreservedOnHistoryNavigation(t *testing.T) {
	model := newTestSkillsModel(t, 2)

	// Record history command
	model.bottom.recordHistory("git status")
	model.bottom.recordHistory("docker ps")

	// Type an in-progress draft
	draftText := "my half-written complex query"
	model.bottom.prompt().SetValue(draftText)

	// Press Up to navigate into history
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(*bubbleModel)
	if model.bottom.prompt().Value() != "docker ps" {
		t.Fatalf("expected 'docker ps' from history, got: %q", model.bottom.prompt().Value())
	}

	// Press Up again
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(*bubbleModel)
	if model.bottom.prompt().Value() != "git status" {
		t.Fatalf("expected 'git status' from history, got: %q", model.bottom.prompt().Value())
	}

	// Press Down to return towards draft
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(*bubbleModel)
	if model.bottom.prompt().Value() != "docker ps" {
		t.Fatalf("expected 'docker ps', got: %q", model.bottom.prompt().Value())
	}

	// Press Down back to the bottom: draft must be restored!
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(*bubbleModel)
	if model.bottom.prompt().Value() != draftText {
		t.Fatalf("expected restored draft %q, got: %q", draftText, model.bottom.prompt().Value())
	}
}

func TestSlashAutocompleteWrapAround(t *testing.T) {
	model := newTestSkillsModel(t, 8)
	model.bottom.prompt().SetValue("/skill ")

	if !model.slashOpen() {
		t.Fatal("expected slash open for /skill ")
	}

	// Move Up at top match (wrap to bottom)
	model.moveSlash(-1)
	matches := model.slashMatches()
	state := model.slashState()
	if state.index != len(matches)-1 {
		t.Fatalf("expected wrapped index %d, got %d", len(matches)-1, state.index)
	}

	// Move Down at bottom match (wrap to top)
	model.moveSlash(1)
	if state.index != 0 {
		t.Fatalf("expected wrapped index 0, got %d", state.index)
	}
}

func TestSlashAutocompleteAlignedColumns(t *testing.T) {
	model := newTestSkillsModel(t, 5)
	model.bottom.prompt().SetValue("/skill ")

	if !model.slashOpen() {
		t.Fatal("expected slash open for /skill ")
	}

	rendered := model.renderSlash(0)
	// Check that checkboxes are consistently placed before names and descriptions use ellipsis
	if !strings.Contains(rendered, "❯ [ ] skill-01") {
		t.Fatalf("expected aligned cursor and checkbox '❯ [ ] skill-01', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "  [ ] skill-02") {
		t.Fatalf("expected aligned unselected row '  [ ] skill-02', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[user   ]") {
		t.Fatalf("expected aligned scope tag '[user   ]', got:\n%s", rendered)
	}
}

func TestSkillsCommandBoundedOutput(t *testing.T) {
	model := newTestSkillsModel(t, 25)
	model.executeCommand("/skills")

	view := model.viewport.View()
	// Should not dump 25 lines into viewport, but bound output and mention more skills
	if strings.Contains(view, "skill-25") {
		t.Fatalf("skill-25 should not be dumped into transcript for large list, got:\n%s", view)
	}
	if !strings.Contains(view, "more skills") {
		t.Fatalf("expected bounded summary 'more skills' in transcript, got:\n%s", view)
	}
}

func TestSkillsPickerCtrlCEscapesModal(t *testing.T) {
	model := newTestSkillsModel(t, 5)
	// Open picker via ctrl+s
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(*bubbleModel)
	if !model.bottom.has(skillsViewID) {
		t.Fatal("expected skills picker open")
	}

	// Press ctrl+c -> must dismiss modal
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(*bubbleModel)
	if model.bottom.has(skillsViewID) {
		t.Fatal("expected ctrl+c to close skills picker")
	}
}

func TestTranscriptOverlayQAndCtrlC(t *testing.T) {
	model := newTestSkillsModel(t, 2)
	model.showTranscript = true

	// 'q' closes transcript
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = updated.(*bubbleModel)
	if model.showTranscript {
		t.Fatal("expected 'q' to close transcript overlay")
	}

	// Re-open and test ctrl+c closes transcript
	model.showTranscript = true
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(*bubbleModel)
	if model.showTranscript {
		t.Fatal("expected ctrl+c to close transcript overlay")
	}
}

func TestMessageHistoryIntegrityOnTurnCancel(t *testing.T) {
	bModel := newTestSkillsModel(t, 1)

	// Simulate starting a turn (user prompt added to messages)
	bModel.messages = append(bModel.messages, model.Message{Role: model.RoleUser, Content: "do something that will be cancelled"})

	// Simulate turn cancellation or error before assistant tokens arrive
	updated, _ := bModel.Update(turnDoneMsg{err: context.Canceled})
	bModel = updated.(*bubbleModel)

	// Invariant: The orphan user prompt must be rolled back so messages never has consecutive user turns
	if len(bModel.messages) != 0 {
		t.Fatalf("expected orphan user message to be rolled back on cancellation, got len=%d: %#v", len(bModel.messages), bModel.messages)
	}
}

func TestQueueClearedOnTurnCancel(t *testing.T) {
	model := newTestSkillsModel(t, 1)
	model.busy = true
	cancelled := false
	model.turnCancel = func() { cancelled = true }
	model.queue = []string{"next queued command 1", "next queued command 2"}

	// User presses ctrl+c while busy
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(*bubbleModel)

	if !cancelled {
		t.Fatal("expected turnCancel to be called")
	}
	if len(model.queue) != 0 {
		t.Fatalf("expected queue to be cleared on cancel, got: %v", model.queue)
	}
}

