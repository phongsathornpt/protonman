package runtime

import "charm.land/bubbles/v2/key"

type paneKeyMap struct {
	Close   key.Binding
	Confirm key.Binding
	Up      key.Binding
	Down    key.Binding
	Nav     key.Binding
}

var paneKeys = paneKeyMap{
	Close:   key.NewBinding(key.WithKeys("esc", "q")),
	Confirm: key.NewBinding(key.WithKeys("enter")),
	Up:      key.NewBinding(key.WithKeys("up", "k")),
	Down:    key.NewBinding(key.WithKeys("down", "j")),
	Nav: key.NewBinding(key.WithKeys(
		"up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G",
	)),
}
