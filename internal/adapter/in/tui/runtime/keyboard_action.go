package runtime

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type composerKeyAction uint8

const (
	composerKeyActionNone composerKeyAction = iota
	composerKeyActionSubmit
	composerKeyActionNewline
)

func (m *bubbleModel) composerAction(message tea.KeyPressMsg) composerKeyAction {
	// Match newline first. Enhanced terminals can distinguish Ctrl+Enter from
	// Enter, while Ctrl+J remains the protocol-safe legacy fallback.
	if key.Matches(message, m.keys.Newline) {
		return composerKeyActionNewline
	}
	if key.Matches(message, m.keys.Submit) {
		return composerKeyActionSubmit
	}
	return composerKeyActionNone
}
