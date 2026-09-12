package keyboardpolicy

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func TestClassifyPrefersNewlineOverSubmit(t *testing.T) {
	binding := key.NewBinding(key.WithKeys("enter"))
	if got := Classify(tea.KeyPressMsg{Code: tea.KeyEnter}, binding, binding); got != Newline {
		t.Fatalf("action = %v, want newline", got)
	}
}
