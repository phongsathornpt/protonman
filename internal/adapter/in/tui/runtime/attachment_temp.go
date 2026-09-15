package runtime

import "strings"

func (p *bottomPane) attachTemporaryImage(path string) {
	if p == nil {
		return
	}
	p.composer.attachments.attachTemporaryImage(&p.composer.input, path)
}

// discardPrompt is for user-abandoned composer drafts. resetPrompt itself keeps
// attachment files alive because submit transfers their ownership into a
// queued/preparing input before resetting the visual composer.
func (m *bubbleModel) discardPrompt() {
	if m == nil || m.panes.bottom == nil {
		return
	}
	m.panes.bottom.composer.attachments.discard()
	m.resetPrompt()
}

func (m *bubbleModel) cleanupPendingImageInput() {
	if m == nil || m.pendingImageInput == nil {
		return
	}
	cleanupQueuedInputAttachments(*m.pendingImageInput)
	m.pendingImageInput = nil
}

func attachmentPathTemporary(path string) bool {
	return strings.TrimSpace(path) != ""
}
