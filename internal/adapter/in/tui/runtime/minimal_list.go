package runtime

import "charm.land/bubbles/v2/list"

func newMinimalList(items []list.Item, delegate list.ItemDelegate, width, height int) list.Model {
	picker := list.New(items, delegate, width, height)
	picker.DisableQuitKeybindings()
	picker.SetShowTitle(false)
	picker.SetShowStatusBar(false)
	picker.SetShowPagination(false)
	picker.SetShowHelp(false)
	return picker
}
