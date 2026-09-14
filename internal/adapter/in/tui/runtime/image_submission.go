package runtime

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/feature/imageprep"
)

type imageSubmissionPreparedMsg struct {
	input   tuiconv.QueuedInput
	message model.Message
	err     error
}

func prepareImageSubmission(input tuiconv.QueuedInput) tea.Cmd {
	input = input.Clone()
	return func() tea.Msg {
		parts := make([]model.ContentPart, 0, len(input.Attachments)+1)
		for _, attachment := range input.Attachments {
			snapshot, err := imageprep.SnapshotLocal(attachment.Path)
			if err != nil {
				return imageSubmissionPreparedMsg{input: input, err: fmt.Errorf("prepare %s: %w", attachment.Placeholder, err)}
			}
			parts = append(parts, model.ContentPart{
				Type:     model.ContentPartImage,
				MIMEType: snapshot.MIMEType,
				Data:     snapshot.Data,
			})
		}
		if text := strings.TrimSpace(input.Text); text != "" {
			parts = append(parts, model.ContentPart{Type: model.ContentPartText, Text: text})
		}
		return imageSubmissionPreparedMsg{
			input: input,
			message: model.Message{
				ID:      model.NewMessageID(),
				Role:    model.RoleUser,
				Content: strings.TrimSpace(input.Text),
				Parts:   parts,
			},
		}
	}
}

func (m *bubbleModel) updateImageSubmissionPrepared(message imageSubmissionPreparedMsg) tea.Cmd {
	m.imagePreparing = false
	if message.err != nil {
		m.appendError(message.err.Error())
		m.restoreSubmissionToComposer(message.input)
		m.requestRelayout()
		return nil
	}
	return m.startTurnMessage(message.message)
}

func (m *bubbleModel) restoreSubmissionToComposer(input tuiconv.QueuedInput) {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return
	}
	prompt := m.panes.bottom.prompt()
	if strings.TrimSpace(prompt.Value()) != "" || len(m.panes.bottom.composer.attachments.localImages) > 0 {
		if m.conversation != nil {
			_ = m.conversation.EnqueueInput(input)
		}
		return
	}
	prompt.SetValue(input.Text)
	prompt.CursorEnd()
	for _, attachment := range input.Attachments {
		m.panes.bottom.attachImage(attachment.Path)
	}
	m.panes.bottom.syncPromptChrome()
}
