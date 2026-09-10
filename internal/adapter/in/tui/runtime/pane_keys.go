package runtime

import "charm.land/bubbles/v2/key"

type paneKeyMap struct {
	Close   key.Binding
	Escape  key.Binding
	Confirm key.Binding
	Tab     key.Binding
	Up      key.Binding
	Down    key.Binding
	Page    key.Binding
	Nav     key.Binding
}

var paneKeys = paneKeyMap{
	Close:   key.NewBinding(key.WithKeys("esc", "q")),
	Escape:  key.NewBinding(key.WithKeys("esc")),
	Confirm: key.NewBinding(key.WithKeys("enter")),
	Tab:     key.NewBinding(key.WithKeys("tab")),
	Up:      key.NewBinding(key.WithKeys("up", "k")),
	Down:    key.NewBinding(key.WithKeys("down", "j")),
	Page:    key.NewBinding(key.WithKeys("pgup", "pgdown")),
	Nav: key.NewBinding(key.WithKeys(
		"up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G",
	)),
}
