//go:build !darwin && !linux

package runtime

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
)

type clipboardImageLoadedMsg struct {
	draftText        string
	draftAttachments int
	path             string
	width            int
	height           int
	err              error
}

func (m *bubbleModel) beginClipboardImagePaste() tea.Cmd {
	if m != nil {
		m.appendError("clipboard image paste is not supported on this platform")
		m.requestRelayout()
	}
	return nil
}

func (m *bubbleModel) updateClipboardImageLoaded(message clipboardImageLoadedMsg) tea.Cmd {
	if m != nil && message.err != nil {
		m.appendError(fmt.Sprintf("clipboard image paste: %v", message.err))
		m.requestRelayout()
	}
	return nil
}
