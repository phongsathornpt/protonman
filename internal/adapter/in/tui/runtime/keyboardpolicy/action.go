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
	CyclePermission
	ToggleTodo
	Transcript
	ToggleSkills
	ToggleModel
	PageUp
	PageDown
)

// Bindings is the semantic view of the TUI keymap used for action
// classification. Runtime execution stays outside this package.
type Bindings struct {
	Submit          key.Binding
	Newline         key.Binding
	CyclePermission key.Binding
	ToggleTodo      key.Binding
	Transcript      key.Binding
	ToggleSkills    key.Binding
	ToggleModel     key.Binding
	PageUp          key.Binding
	PageDown        key.Binding
}

// Priority classifies shortcuts that preempt non-blocking panes. Permission
// cycling is included here but runtime intentionally executes it even before a
// blocking pane gets first refusal.
func (b Bindings) Priority(message tea.KeyPressMsg) Action {
	switch {
	case key.Matches(message, b.CyclePermission):
		return CyclePermission
	case key.Matches(message, b.ToggleTodo):
		return ToggleTodo
	case key.Matches(message, b.Transcript):
		return Transcript
	case key.Matches(message, b.ToggleSkills):
		return ToggleSkills
	case key.Matches(message, b.ToggleModel):
		return ToggleModel
	default:
		return None
	}
}

// Navigation classifies global viewport navigation after the active pane has
// had an opportunity to consume the key.
func (b Bindings) Navigation(message tea.KeyPressMsg) Action {
	switch {
	case key.Matches(message, b.PageUp):
		return PageUp
	case key.Matches(message, b.PageDown):
		return PageDown
	default:
		return None
	}
}

// Composer classifies prompt actions and preserves newline-before-submit
// precedence for terminals where the bindings can overlap.
func (b Bindings) Composer(message tea.KeyPressMsg) Action {
	if key.Matches(message, b.Newline) {
		return Newline
	}
	if key.Matches(message, b.Submit) {
		return Submit
	}
	return None
}

// Classify preserves the existing focused composer API for callers and tests.
func Classify(message tea.KeyPressMsg, newline, submit key.Binding) Action {
	return (Bindings{Newline: newline, Submit: submit}).Composer(message)
}
