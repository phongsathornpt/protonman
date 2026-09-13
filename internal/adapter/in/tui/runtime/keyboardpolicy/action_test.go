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

func TestBindingsClassifyActionScopes(t *testing.T) {
	bindings := Bindings{
		Submit:          key.NewBinding(key.WithKeys("enter")),
		Newline:         key.NewBinding(key.WithKeys("ctrl+j")),
		CyclePermission: key.NewBinding(key.WithKeys("shift+tab")),
		ToggleTodo:      key.NewBinding(key.WithKeys("ctrl+o")),
		Transcript:      key.NewBinding(key.WithKeys("ctrl+t")),
		ToggleSkills:    key.NewBinding(key.WithKeys("ctrl+s")),
		ToggleModel:     key.NewBinding(key.WithKeys("ctrl+p")),
		PageUp:          key.NewBinding(key.WithKeys("pgup")),
		PageDown:        key.NewBinding(key.WithKeys("pgdown")),
	}

	priority := []struct {
		key  tea.KeyPressMsg
		want Action
	}{
		{tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, CyclePermission},
		{tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}, ToggleTodo},
		{tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}, Transcript},
		{tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}, ToggleSkills},
		{tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}, ToggleModel},
	}
	for _, tt := range priority {
		if got := bindings.Priority(tt.key); got != tt.want {
			t.Fatalf("priority action = %v, want %v", got, tt.want)
		}
	}

	if got := bindings.Navigation(tea.KeyPressMsg{Code: tea.KeyPgUp}); got != PageUp {
		t.Fatalf("page up action = %v", got)
	}
	if got := bindings.Navigation(tea.KeyPressMsg{Code: tea.KeyPgDown}); got != PageDown {
		t.Fatalf("page down action = %v", got)
	}
	if got := bindings.Composer(tea.KeyPressMsg{Code: tea.KeyEnter}); got != Submit {
		t.Fatalf("submit action = %v", got)
	}
	if got := bindings.Composer(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}); got != Newline {
		t.Fatalf("newline action = %v", got)
	}
}
