package runtime

import (
	"encoding/base64"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/feature/imageprep"
)

type imageSubmissionPreparedMsg struct {
	preparationID uint64
	input         tuiconv.QueuedInput
	message       model.Message
	err           error
}

func prepareImageSubmission(preparationID uint64, input tuiconv.QueuedInput) tea.Cmd {
	input = input.Clone()
	return func() tea.Msg {
		parts := make([]model.ContentPart, 0, len(input.Attachments)+1)
		for _, attachment := range input.Attachments {
			snapshot, err := imageprep.SnapshotLocal(attachment.Path)
			if err != nil {
				return imageSubmissionPreparedMsg{
					preparationID: preparationID,
					input:         input,
					err:           fmt.Errorf("prepare %s: %w", attachment.Placeholder, err),
				}
			}
			parts = append(parts, model.ContentPart{
				Type:     model.ContentPartImage,
				MIMEType: snapshot.MIMEType,
				Data:     base64.StdEncoding.EncodeToString(snapshot.Bytes),
			})
		}
		if text := strings.TrimSpace(input.Text); text != "" {
			parts = append(parts, model.ContentPart{Type: model.ContentPartText, Text: text})
		}
		return imageSubmissionPreparedMsg{
			preparationID: preparationID,
			input:         input,
			message: model.Message{
				ID:      model.NewMessageID(),
				Role:    model.RoleUser,
				Content: strings.TrimSpace(input.Text),
				Parts:   parts,
			},
		}
	}
}

func (m *bubbleModel) beginImagePreparation(input tuiconv.QueuedInput) tea.Cmd {
	m.imagePreparationID++
	preparationID := m.imagePreparationID
	pending := input.Clone()
	m.pendingImageInput = &pending
	m.imagePreparing = true
	m.activity = "preparing image"
	m.requestRelayout()
	return prepareImageSubmission(preparationID, input)
}

func (m *bubbleModel) updateImageSubmissionPrepared(message imageSubmissionPreparedMsg) tea.Cmd {
	if message.preparationID != m.imagePreparationID {
		// A canceled or superseded preparation is allowed to finish its local
		// work, but it must never be able to start a model turn afterwards.
		return nil
	}
	m.imagePreparing = false
	m.pendingImageInput = nil
	m.activity = "ready"
	if message.err != nil {
		m.appendError(message.err.Error())
		m.restoreSubmissionToComposer(message.input)
		m.requestRelayout()
		return nil
	}
	// SnapshotLocal made the model message self-contained. Temporary clipboard
	// files are no longer needed once every attachment has been snapshotted.
	cleanupQueuedInputAttachments(message.input)
	if history := submissionHistoryText(message.input); history != "" {
		m.panes.bottom.recordHistory(history)
	}
	m.appendUser(submissionDisplayText(message.input))
	return m.startTurnMessage(message.message)
}

func (m *bubbleModel) cancelImagePreparation() bool {
	if m == nil || !m.imagePreparing {
		return false
	}
	m.imagePreparationID++
	m.imagePreparing = false
	m.activity = "ready"
	var pending *tuiconv.QueuedInput
	if m.pendingImageInput != nil {
		clone := m.pendingImageInput.Clone()
		pending = &clone
	}
	m.pendingImageInput = nil
	if pending != nil {
		m.restoreCanceledImageSubmission(*pending)
	}
	m.requestRelayout()
	return true
}

func (m *bubbleModel) restoreCanceledImageSubmission(input tuiconv.QueuedInput) {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		cleanupQueuedInputAttachments(input)
		return
	}
	prompt := m.panes.bottom.prompt()
	if strings.TrimSpace(prompt.Value()) != "" || len(m.panes.bottom.composer.attachments.localImages) > 0 {
		cleanupQueuedInputAttachments(input)
		return
	}
	m.restoreSubmissionToComposer(input)
}

func (m *bubbleModel) restoreSubmissionToComposer(input tuiconv.QueuedInput) {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		cleanupQueuedInputAttachments(input)
		return
	}
	prompt := m.panes.bottom.prompt()
	if strings.TrimSpace(prompt.Value()) != "" || len(m.panes.bottom.composer.attachments.localImages) > 0 {
		if m.conversation == nil || !m.conversation.EnqueueInput(input) {
			cleanupQueuedInputAttachments(input)
		}
		return
	}
	prompt.SetValue(input.Text)
	prompt.CursorEnd()
	for _, attachment := range input.Attachments {
		if attachment.Temporary {
			m.panes.bottom.composer.attachments.attachTemporaryImage(prompt, attachment.Path)
			continue
		}
		m.panes.bottom.attachImage(attachment.Path)
	}
	m.panes.bottom.syncPromptChrome()
}
