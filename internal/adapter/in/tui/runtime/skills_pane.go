package runtime

import (
	"fmt"
	"io"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

const skillsViewID = "skills"

var skillPaneToggleKey = key.NewBinding(key.WithKeys("space", "t"))

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

type skillSetupDelegate struct{}

func (skillSetupDelegate) Height() int                         { return 1 }
func (skillSetupDelegate) Spacing() int                        { return 0 }
func (skillSetupDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (skillSetupDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(skillListItem)
	if !ok {
		return
	}
	prefix, style := "  ", bodyStyle
	if index == m.Index() {
		prefix, style = "> ", brandStyle
	}
	_, _ = fmt.Fprint(w, prefix+style.Render(truncateWithEllipsis(entry.Title(), maxInt(1, m.Width()-2))))
}

type skillsPaneView struct {
	picker      list.Model
	initialized bool
}

func (*skillsPaneView) ID() string                             { return skillsViewID }
func (*skillsPaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func (v *skillsPaneView) ensurePicker(ctx paneRenderContext) {
	if v.initialized {
		return
	}
	items := skillListItems(ctx.skillItems)
	v.picker = newMinimalList(items, skillSetupDelegate{}, skillsListWidth(ctx), skillsListHeight(ctx))
	v.picker.InfiniteScrolling = true
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
	v.syncTitle(ctx)
	active := 0
	for _, skill := range ctx.skillItems {
		if skill.active {
			active++
		}
	}
	help := ""
	if layoutModeForHeight(ctx.height) != layoutTiny {
		help = paneKeyboardHelp(ctx.width-4, "↑/↓", "Navigate", "space", "Toggle", "/", "Filter", "esc", "Go Back")
	}
	items := v.picker.VisibleItems()
	start, end := paneWindow(len(items), v.picker.Index(), 7, layoutModeForHeight(ctx.height))
	listRows := make([]string, 0, end-start+1)
	if v.picker.SettingFilter() || v.picker.IsFiltered() {
		listRows = append(listRows, mutedStyle.Render("Search: ")+userStyle.Render(v.picker.FilterValue()))
	}
	for index := start; index < end; index++ {
		item, ok := items[index].(skillListItem)
		if !ok {
			continue
		}
		prefix, style := "  ", bodyStyle
		if index == v.picker.Index() {
			prefix, style = "> ", brandStyle
		}
		listRows = append(listRows, prefix+style.Render(truncateWithEllipsis(item.Title(), maxInt(1, ctx.width-8))))
	}
	status := fmt.Sprintf("%d/%d active", active, len(ctx.skillItems))
	if selected, ok := v.picker.SelectedItem().(skillListItem); ok {
		status = selected.name + " · " + status
	}
	rows := paneSection("Skills", listRows, help, status, ctx.width)
	return renderModalRows(ctx, accentAssistant, rows)
}
func (v *skillsPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	v.ensurePicker(ctx)
	if !v.initialized || len(ctx.skillItems) == 0 {
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: skillsViewID}}
	}
	switch {
	case key.Matches(message, skillPaneToggleKey):
		selected, ok := v.picker.SelectedItem().(skillListItem)
		if !ok {
			return paneKeyResult{handled: true}
		}
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionToggleSkill, skillName: selected.name}}
	case message.Text >= "1" && message.Text <= "9":
		index := int(message.Text[0] - '1')
		if index < len(v.picker.Items()) {
			v.picker.Select(index)
			v.syncTitle(ctx)
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneKeys.Confirm):
		if !v.picker.SettingFilter() {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: skillsViewID}}
		}
	case key.Matches(message, paneKeys.Escape):
		if !v.picker.SettingFilter() && !v.picker.IsFiltered() {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: skillsViewID}}
		}
	case message.Text == "q":
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
