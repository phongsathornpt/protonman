package runtime

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

const skillsViewID = "skills"

type skillListItem struct {
	name   string
	active bool
}

func (i skillListItem) FilterValue() string { return i.name }
func (i skillListItem) Description() string { return "" }

func (i skillListItem) Title() string {
	if i.active {
		return "[x] " + i.name
	}
	return "[ ] " + i.name
}

type skillsPaneView struct {
	picker      list.Model
	initialized bool
}

func (*skillsPaneView) ID() string             { return skillsViewID }
func (*skillsPaneView) ReplacesComposer() bool { return true }

func (v *skillsPaneView) ensurePicker(m *bubbleModel) {
	if v.initialized || m == nil || m.skills == nil {
		return
	}
	items := skillListItems(m)
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	v.picker = list.New(items, delegate, skillsListWidth(m), skillsListHeight(m))
	v.picker.InfiniteScrolling = true
	v.picker.DisableQuitKeybindings()
	v.picker.SetStatusBarItemName("skill", "skills")
	v.picker.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
		}
	}
	v.initialized = true
	v.syncTitle(m)
}
func skillListItems(m *bubbleModel) []list.Item {
	if m == nil || m.skills == nil {
		return nil
	}
	skills := m.skills.List()
	items := make([]list.Item, 0, len(skills))
	for _, skill := range skills {
		items = append(items, skillListItem{
			name:   skill.Name,
			active: m.skills.IsActivated(skill.Name),
		})
	}
	return items
}

func skillsListWidth(m *bubbleModel) int {
	return maxInt(12, m.layout.width-8)
}

func skillsListHeight(m *bubbleModel) int {
	return maxInt(4, min(8, m.layout.height-6))
}
func (v *skillsPaneView) syncTitle(m *bubbleModel) {
	if !v.initialized || m == nil || m.skills == nil {
		return
	}
	active := 0
	for _, skill := range m.skills.List() {
		if m.skills.IsActivated(skill.Name) {
			active++
		}
	}
	count := len(v.picker.Items())
	v.picker.Title = fmt.Sprintf("Skills · %d/%d active", active, count)
}

func (v *skillsPaneView) Render(m *bubbleModel) string {
	v.ensurePicker(m)
	if !v.initialized {
		return ""
	}
	v.picker.SetSize(skillsListWidth(m), skillsListHeight(m))
	v.configureDensity(m)
	v.syncTitle(m)
	return renderModalRows(m, accentAssistant, strings.Split(v.picker.View(), "\n"))
}

func (v *skillsPaneView) configureDensity(m *bubbleModel) {
	v.picker.SetShowStatusBar(false)
	// Keep pagination presentation hidden; the list component still owns navigation.
	v.picker.SetShowPagination(false)
	v.picker.SetShowHelp(m != nil && layoutModeForHeight(m.layout.height) != layoutTiny)
}
func (v *skillsPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	v.ensurePicker(m)
	if !v.initialized || m == nil || m.skills == nil {
		if m != nil {
			m.panes.bottom.remove(skillsViewID)
		}
		return true, nil
	}
	if message.String() == "ctrl+s" {
		m.panes.bottom.remove(skillsViewID)
		return true, nil
	}

	switch message.String() {
	case "space", "t":
		return true, v.toggleSelected(m)
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		index := int(message.String()[0] - '1')
		if index < len(v.picker.Items()) {
			v.picker.Select(index)
			v.syncTitle(m)
		}
		return true, nil
	case "enter":
		if !v.picker.SettingFilter() {
			m.panes.bottom.remove(skillsViewID)
			return true, nil
		}
	case "esc":
		if !v.picker.SettingFilter() && !v.picker.IsFiltered() {
			m.panes.bottom.remove(skillsViewID)
			return true, nil
		}
	case "q":
		if !v.picker.SettingFilter() {
			m.panes.bottom.remove(skillsViewID)
			return true, nil
		}
	}

	updated, cmd := v.picker.Update(message)
	v.picker = updated
	v.syncTitle(m)
	return true, cmd
}
func (v *skillsPaneView) toggleSelected(m *bubbleModel) tea.Cmd {
	selected, ok := v.picker.SelectedItem().(skillListItem)
	if !ok {
		return nil
	}
	_, _ = m.skills.Toggle(selected.name)
	selected.active = m.skills.IsActivated(selected.name)
	cmd := v.picker.SetItem(v.picker.GlobalIndex(), selected)
	v.syncTitle(m)
	return cmd
}
