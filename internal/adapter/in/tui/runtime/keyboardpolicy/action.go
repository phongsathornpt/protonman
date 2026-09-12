package keyboardpolicy

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type Action uint8

const (
	None Action = iota
	Submit
	Newline
)

// Classify preserves the intentional newline-before-submit precedence.
func Classify(message tea.KeyPressMsg, newline, submit key.Binding) Action {
	if key.Matches(message, newline) {
		return Newline
	}
	if key.Matches(message, submit) {
		return Submit
	}
	return None
}
