package paneutil

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
)

type KeyMap struct {
	Close, Escape, Confirm, Tab, Up, Down, Page, Nav key.Binding
}

var Keys = KeyMap{
	Close:   key.NewBinding(key.WithKeys("esc", "q")),
	Escape:  key.NewBinding(key.WithKeys("esc")),
	Confirm: key.NewBinding(key.WithKeys("enter")),
	Tab:     key.NewBinding(key.WithKeys("tab")),
	Up:      key.NewBinding(key.WithKeys("up", "k")),
	Down:    key.NewBinding(key.WithKeys("down", "j")),
	Page:    key.NewBinding(key.WithKeys("pgup", "pgdown")),
	Nav:     key.NewBinding(key.WithKeys("up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G")),
}

func NewMinimalList(items []list.Item, delegate list.ItemDelegate, width, height int) list.Model {
	picker := list.New(items, delegate, width, height)
	picker.DisableQuitKeybindings()
	picker.SetShowTitle(false)
	picker.SetShowStatusBar(false)
	picker.SetShowPagination(false)
	picker.SetShowHelp(false)
	return picker
}
