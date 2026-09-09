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

func (v *skillsPaneView) ensurePicker(ctx paneRenderContext) {
	if v.initialized {
		return
	}
	items := skillListItems(ctx.skillItems)
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	v.picker = list.New(items, delegate, skillsListWidth(ctx), skillsListHeight(ctx))
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
	v.syncTitle(ctx)
}
func skillListItems(items []skillListItem) []list.Item {
	out := make([]list.Item, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func skillsListWidth(ctx paneRenderContext) int {
	return maxInt(12, ctx.width-8)
}

func skillsListHeight(ctx paneRenderContext) int {
	return maxInt(4, min(8, ctx.height-6))
}
func (v *skillsPaneView) syncTitle(ctx paneRenderContext) {
	if !v.initialized {
		return
	}
	active := 0
	for _, skill := range ctx.skillItems {
		if skill.active {
			active++
		}
	}
	count := len(v.picker.Items())
	v.picker.Title = fmt.Sprintf("Skills · %d/%d active", active, count)
}

func (v *skillsPaneView) Render(ctx paneRenderContext) string {
	v.ensurePicker(ctx)
	if !v.initialized {
		return ""
	}
	v.picker.SetSize(skillsListWidth(ctx), skillsListHeight(ctx))
	v.configureDensity(ctx)
	v.syncTitle(ctx)
	return renderModalRows(ctx, accentAssistant, strings.Split(v.picker.View(), "\n"))
}

func (v *skillsPaneView) configureDensity(ctx paneRenderContext) {
	v.picker.SetShowStatusBar(false)
	// Keep pagination presentation hidden; the list component still owns navigation.
	v.picker.SetShowPagination(false)
	v.picker.SetShowHelp(layoutModeForHeight(ctx.height) != layoutTiny)
}
func (v *skillsPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	v.ensurePicker(ctx)
	if !v.initialized || len(ctx.skillItems) == 0 {
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: skillsViewID}}
	}
	if message.String() == "ctrl+s" {
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: skillsViewID}}
	}

	switch message.String() {
	case "space", "t":
		selected, ok := v.picker.SelectedItem().(skillListItem)
		if !ok {
			return paneKeyResult{handled: true}
		}
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionToggleSkill, skillName: selected.name}}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		index := int(message.String()[0] - "1"[0])
		if index < len(v.picker.Items()) {
			v.picker.Select(index)
			v.syncTitle(ctx)
		}
		return paneKeyResult{handled: true}
	case "enter":
		if !v.picker.SettingFilter() {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: skillsViewID}}
		}
	case "esc":
		if !v.picker.SettingFilter() && !v.picker.IsFiltered() {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: skillsViewID}}
		}
	case "q":
		if !v.picker.SettingFilter() {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: skillsViewID}}
		}
	}

	updated, cmd := v.picker.Update(message)
	v.picker = updated
	v.syncTitle(ctx)
	return paneKeyResult{handled: true, cmd: cmd}
}

func (v *skillsPaneView) refreshItems(ctx paneRenderContext) tea.Cmd {
	cmd := v.picker.SetItems(skillListItems(ctx.skillItems))
	v.syncTitle(ctx)
	return cmd
}
