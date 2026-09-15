package runtime

import (
	"strings"
	"testing"

	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestSubmissionHistoryTextNeverPersistsImageMarkers(t *testing.T) {
	input := tuiconv.QueuedInput{
		Text: "inspect this layout",
		Attachments: []tuiconv.Attachment{{
			Placeholder: "[Image #1]",
			Path:        "/tmp/layout.png",
		}},
	}
	if got := submissionHistoryText(input); got != "inspect this layout" {
		t.Fatalf("history text = %q, want text only", got)
	}
	input.Text = ""
	if got := submissionHistoryText(input); got != "" {
		t.Fatalf("image-only history text = %q, want empty", got)
	}
}

func TestHistoryNavigationDoesNotReplaceAttachmentDraft(t *testing.T) {
	pane := newBottomPane(true, true)
	pane.recordHistory("previous prompt")
	pane.prompt().SetValue("current")
	pane.prompt().CursorEnd()
	pane.attachImage("/tmp/current.png")
	before := pane.prompt().Value()
	beforePos := pane.composer.historyPos

	pane.historyPrevious()

	if got := pane.prompt().Value(); got != before {
		t.Fatalf("attachment draft changed to %q, want %q", got, before)
	}
	if pane.composer.historyPos != beforePos {
		t.Fatalf("history position = %d, want %d", pane.composer.historyPos, beforePos)
	}
	if len(pane.composer.attachments.localImages) != 1 {
		t.Fatalf("attachments = %d, want 1", len(pane.composer.attachments.localImages))
	}
}

func TestCancelImagePreparationInvalidatesStaleCompletion(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	input := tuiconv.QueuedInput{
		Text: "inspect screenshot",
		Attachments: []tuiconv.Attachment{{
			Placeholder: "[Image #1]",
			Path:        "/tmp/screenshot.png",
		}},
	}
	pending := input.Clone()
	m.imagePreparing = true
	m.imagePreparationID = 41
	m.pendingImageInput = &pending

	if !m.cancelImagePreparation() {
		t.Fatal("cancelImagePreparation() = false")
	}
	if m.imagePreparing || m.pendingImageInput != nil {
		t.Fatalf("preparation state not cleared: preparing=%v pending=%v", m.imagePreparing, m.pendingImageInput)
	}
	if m.imagePreparationID != 42 {
		t.Fatalf("preparation id = %d, want 42", m.imagePreparationID)
	}
	if got := m.panes.bottom.prompt().Value(); !strings.Contains(got, "inspect screenshot") || !strings.Contains(got, "[Image #1]") {
		t.Fatalf("canceled draft was not restored: %q", got)
	}
	if got := len(m.panes.bottom.composer.history); got != 0 {
		t.Fatalf("canceled image turn wrote %d history entries, want 0", got)
	}

	stale := imageSubmissionPreparedMsg{
		preparationID: 41,
		input:         input,
		message: model.Message{
			ID:   model.NewMessageID(),
			Role: model.RoleUser,
			Parts: []model.ContentPart{{
				Type: model.ContentPartImage, MIMEType: "image/png", Data: "iVBORw0KGgo=",
			}},
		},
	}
	if cmd := m.updateImageSubmissionPrepared(stale); cmd != nil {
		t.Fatalf("stale preparation returned command: %v", cmd)
	}
	if m.busy {
		t.Fatal("stale preparation started a model turn")
	}
	if got := len(m.panes.bottom.composer.history); got != 0 {
		t.Fatalf("stale completion wrote %d history entries, want 0", got)
	}
}
