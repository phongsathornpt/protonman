package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"strings"
)

const maxSlashRows = 6

const slashViewID = "slash"

type slashCommand = slashview.Command

var slashCatalog = slashview.Catalog(agent.ProfileList("|"))

type slashListItem struct{ command slashCommand }

func (i slashListItem) FilterValue() string {
	return i.command.Name + " " + strings.Join(i.command.Aliases, " ") + " " + i.command.Description
}
func (i slashListItem) Title() string {
	prefix := i.command.PrefixTag
	if prefix != "" {
		prefix += " "
	}
	return prefix + i.command.Name
}
func (i slashListItem) Description() string {
	if i.command.Scope != "" {
		return i.command.Description + " · " + i.command.Scope
	}
	return i.command.Description
}

type slashPaneView struct {
	picker  list.Model
	ready   bool
	matches []slashCommand
}

func (*slashPaneView) ID() string             { return slashViewID }
func (*slashPaneView) ReplacesComposer() bool { return false }

func (v *slashPaneView) sync(m *bubbleModel) {
	matches := m.slashMatches()
	v.matches = append(v.matches[:0], matches...)
	items := make([]list.Item, 0, len(matches))
	for _, command := range matches {
		items = append(items, slashListItem{command: command})
	}
	if !v.ready {
		delegate := list.NewDefaultDelegate()
		delegate.SetSpacing(0)
		v.picker = list.New(items, delegate, maxInt(20, m.width-4), maxInt(4, minInt(12, m.height/2)))
		v.picker.DisableQuitKeybindings()
		v.picker.SetFilteringEnabled(false)
		v.picker.SetShowTitle(false)
		v.picker.SetShowStatusBar(false)
		v.picker.SetShowPagination(false)
		v.picker.SetShowHelp(false)
		v.picker.InfiniteScrolling = true
		v.ready = true
	} else {
		_ = v.picker.SetItems(items)
	}
	if len(matches) == 0 {
		return
	}
	selected := maxInt(0, minInt(v.picker.Index(), len(matches)-1))
	v.picker.Select(selected)
}

func (v *slashPaneView) Render(m *bubbleModel) string {
	v.sync(m)
	if len(v.matches) == 0 {
		return ""
	}
	v.picker.SetSize(maxInt(20, m.width-4), maxInt(4, minInt(12, m.height/2)))
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = layoutModeForHeight(m.height) == layoutNormal
	v.picker.SetDelegate(delegate)
	return v.picker.View()
}

func (v *slashPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	v.sync(m)
	switch message.String() {
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return true, cmd
	case "tab":
		_, command := m.acceptSlash(false)
		return true, command
	case "enter":
		_, command := m.acceptSlash(true)
		return true, command
	case "esc":
		m.bottom.remove(slashViewID)
		return true, nil
	default:
		return false, nil
	}
}
