package runtime

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

// --- Shortcuts Pane ---

const shortcutsViewID = "shortcuts"

type shortcutsPaneView struct{}

func (*shortcutsPaneView) ID() string                             { return shortcutsViewID }
func (*shortcutsPaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func shortcutRow(binding key.Binding) string {
	help := binding.Help()
	desc := help.Desc
	if desc != "" {
		desc = strings.ToUpper(desc[:1]) + desc[1:]
	}
	return userStyle.Render(help.Key) + mutedStyle.Render("  "+desc)
}

func (*shortcutsPaneView) Render(ctx paneRenderContext) string {
	keys := newBubbleKeyMap()
	setComposerNewlineHelp(&keys.Newline, ctx.keyboardCapability)
	rows := []string{
		shortcutRow(keys.Submit),
		shortcutRow(keys.Newline),
		shortcutRow(keys.ToggleModel),
		shortcutRow(keys.Transcript),
		shortcutRow(keys.ToggleTodo),
		shortcutRow(keys.ToggleSkills),
		shortcutRow(keys.CyclePermission),
		shortcutRow(keys.Quit),
	}
	help := paneKeyboardHelp(ctx.width-4, "esc/?", "Go Back")
	return renderModalRows(ctx, accentAssistant, paneSection("Shortcuts", rows, help, "", ctx.width))
}

func (*shortcutsPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	if key.Matches(message, paneutil.Keys.Close, paneutil.Keys.Confirm) || message.Text == "?" {
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: shortcutsViewID}}
	}
	return paneKeyResult{handled: true}
}

func (m *bubbleModel) openShortcutsPane() {
	if m.panes.bottom.has(shortcutsViewID) {
		m.panes.bottom.remove(shortcutsViewID)
		m.requestRelayout()
		return
	}
	m.panes.bottom.push(&shortcutsPaneView{})
	m.requestRelayout()
}

// --- Low Concurrency Pane ---

const lowConcurrencyViewID = "low-concurrency"

type lowConcurrencyPaneView struct {
	index     int
	effective bool
	modelID   string
}

func (*lowConcurrencyPaneView) ID() string                             { return lowConcurrencyViewID }
func (*lowConcurrencyPaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func (v *lowConcurrencyPaneView) Render(ctx paneRenderContext) string {
	choices := []struct {
		label string
		desc  string
	}{
		{"Auto", "use provider/model recommendation"},
		{"On", "force low concurrency for this model"},
		{"Off", "disable low concurrency"},
	}
	rows := make([]string, 0, len(choices)+1)
	for i, choice := range choices {
		marker := "  "
		style := mutedStyle
		if i == v.index {
			marker = "> "
			style = userStyle
		}
		rows = append(rows, marker+style.Render(choice.label)+"  "+mutedStyle.Render(choice.desc))
	}
	state := "off"
	if v.effective {
		state = "on"
	}
	if v.modelID != "" {
		rows = append(rows, "", mutedStyle.Render(fmt.Sprintf("Effective  %s · %s", state, v.modelID)))
	} else {
		rows = append(rows, "", mutedStyle.Render("Effective  off · no active model"))
	}
	help := paneKeyboardHelp(ctx.width-4, "↑/↓", "Navigate", "enter", "Apply", "esc/q", "Go Back")
	return renderModalRows(ctx, accentAssistant, paneSection("Low Concurrency", rows, help, "", ctx.width))
}

func (v *lowConcurrencyPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch {
	case key.Matches(message, paneutil.Keys.Close):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: lowConcurrencyViewID}}
	case key.Matches(message, paneutil.Keys.Up):
		if v.index > 0 {
			v.index--
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Down):
		if v.index < 2 {
			v.index++
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Confirm):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionSetLowConcurrency, lowConcurrency: model.LowConcurrencySetting(v.index)}}
	default:
		return paneKeyResult{handled: true}
	}
}

func (m *bubbleModel) openLowConcurrencyPane() {
	if m.panes.bottom.has(lowConcurrencyViewID) {
		m.panes.bottom.remove(lowConcurrencyViewID)
		m.requestRelayout()
		return
	}
	m.panes.bottom.push(&lowConcurrencyPaneView{index: int(m.lowConcurrencyMode), effective: m.lowConcurrencyEffective(), modelID: m.activeModel})
	m.requestRelayout()
}

// --- Permission Mode Pane ---

const permissionModeViewID = "permission-mode"

type permissionModeChoice uint8

const (
	permissionModeAsk permissionModeChoice = iota
	permissionModePlan
	permissionModeAlwaysApprove
)

type permissionModePaneView struct{ index int }

func (*permissionModePaneView) ID() string                             { return permissionModeViewID }
func (*permissionModePaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func (v *permissionModePaneView) Render(ctx paneRenderContext) string {
	labels := []string{"Ask", "Plan", "Always Approve"}
	rows := make([]string, 0, len(labels))
	for i, label := range labels {
		marker := "  "
		style := mutedStyle
		if i == v.index {
			marker = "> "
			style = userStyle
		}
		rows = append(rows, marker+style.Render(label))
	}
	help := paneKeyboardHelp(ctx.width-4, "↑/↓", "Navigate", "enter", "Select", "esc/q", "Go Back")
	return renderModalRows(ctx, accentAssistant, paneSection("Permission Mode", rows, help, "", ctx.width))
}

func (v *permissionModePaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch {
	case key.Matches(message, paneutil.Keys.Close):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: permissionModeViewID}}
	case key.Matches(message, paneutil.Keys.Up):
		if v.index > 0 {
			v.index--
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Down):
		if v.index < 2 {
			v.index++
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Confirm):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionSetPermissionMode, permissionMode: permissionModeChoice(v.index)}}
	default:
		return paneKeyResult{handled: true}
	}
}

func currentPermissionModeChoice(m *bubbleModel) permissionModeChoice {
	if m != nil && m.planMode {
		return permissionModePlan
	}
	if m != nil && m.service != nil && m.service.Mode() == permission.ModeAlwaysApprove {
		return permissionModeAlwaysApprove
	}
	return permissionModeAsk
}

func (m *bubbleModel) syncPermissionModePane() {
	if view, _ := m.panes.bottom.find(permissionModeViewID).(*permissionModePaneView); view != nil {
		view.index = int(currentPermissionModeChoice(m))
	}
}

func (m *bubbleModel) openPermissionModePane() {
	if m.panes.bottom.has(permissionModeViewID) {
		m.panes.bottom.remove(permissionModeViewID)
		m.requestRelayout()
		return
	}
	m.panes.bottom.push(&permissionModePaneView{index: int(currentPermissionModeChoice(m))})
	m.requestRelayout()
}

func (m *bubbleModel) applyPermissionModeChoice(choice permissionModeChoice) {
	m.setPlanEnabled(false)
	switch choice {
	case permissionModePlan:
		_ = m.setPermissionMode(permission.ModeAsk)
		m.setPlanEnabled(true)
	case permissionModeAlwaysApprove:
		_ = m.setPermissionMode(permission.ModeAlwaysApprove)
	default:
		_ = m.setPermissionMode(permission.ModeAsk)
	}
	m.syncPermissionModePane()
	m.requestRelayout()
}

func (m *bubbleModel) permissionModeLabel() string {
	if m.planMode {
		return "plan"
	}
	if m.service == nil {
		return "ask"
	}
	mode := strings.TrimSpace(m.service.Mode().String())
	if mode == "always-approve" {
		return "auto"
	}
	if mode == "deny" {
		return "deny"
	}
	return "ask"
}

// --- Skills Pane ---

const skillsViewID = "skills"

var skillPaneToggleKey = key.NewBinding(key.WithKeys("space", "t"))

type skillListItem struct {
	name        string
	description string
	scope       string
	active      bool
}

func (i skillListItem) FilterValue() string {
	if i.description != "" {
		return i.name + " " + i.description
	}
	return i.name
}
func (i skillListItem) Description() string { return i.description }

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
	v.picker = paneutil.NewMinimalList(items, skillSetupDelegate{}, skillsListWidth(ctx), skillsListHeight(ctx))
	v.picker.InfiniteScrolling = true
	v.picker.SetStatusBarItemName("skill", "skills")
	v.picker.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("enter/space", "toggle")),
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
		help = paneKeyboardHelp(ctx.width-4, "↑/↓", "Navigate", "enter/space", "Toggle", "/", "Filter", "esc", "Close")
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
	case key.Matches(message, skillPaneToggleKey, paneutil.Keys.Confirm):
		if !v.picker.SettingFilter() {
			selected, ok := v.picker.SelectedItem().(skillListItem)
			if !ok {
				return paneKeyResult{handled: true}
			}
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionToggleSkill, skillName: selected.name}}
		}
	case message.Text >= "1" && message.Text <= "9":
		index := int(message.Text[0] - '1')
		if index < len(v.picker.Items()) {
			v.picker.Select(index)
			v.syncTitle(ctx)
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneutil.Keys.Escape):
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

// --- Session Resume Pane ---

const sessionResumeViewID = "session-resume"

type sessionResumeDelegate struct{}

func (sessionResumeDelegate) Height() int                         { return 1 }
func (sessionResumeDelegate) Spacing() int                        { return 0 }
func (sessionResumeDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (sessionResumeDelegate) Render(io.Writer, list.Model, int, list.Item) {}

type sessionListItem struct {
	summary   app.SessionSummary
	isCurrent bool
}

func (s sessionListItem) FilterValue() string {
	return s.summary.ID + " " + s.summary.WorkspaceName + " " + s.summary.AgentProfile + " " + s.summary.Preview
}

func (s sessionListItem) Title() string {
	return s.summary.ID
}

func (s sessionListItem) Description() string {
	return s.summary.Preview
}

type sessionResumePaneView struct {
	picker      list.Model
	items       []sessionListItem
	initialized bool
}

func (*sessionResumePaneView) ID() string                             { return sessionResumeViewID }
func (*sessionResumePaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func (v *sessionResumePaneView) selectedItem() (sessionListItem, bool) {
	if v == nil || !v.initialized || len(v.items) == 0 {
		return sessionListItem{}, false
	}
	item, ok := v.picker.SelectedItem().(sessionListItem)
	return item, ok
}

const maxSessionResumeRows = 7

func sessionResumeListWidth(ctx paneRenderContext) int {
	return maxInt(12, ctx.width-8)
}

func sessionResumeListHeight(ctx paneRenderContext) int {
	return maxInt(4, min(maxSessionResumeRows, ctx.height-6))
}

func (v *sessionResumePaneView) Render(ctx paneRenderContext) string {
	if !v.initialized || len(v.items) == 0 {
		rows := []string{mutedStyle.Render("No sessions found.")}
		help := paneKeyboardHelp(ctx.width-4, "esc/q", "Go Back")
		return renderModalRows(ctx, accentAssistant, paneSection("Sessions", rows, help, "0 sessions", ctx.width))
	}
	v.picker.SetSize(sessionResumeListWidth(ctx), sessionResumeListHeight(ctx))
	help := paneKeyboardHelp(ctx.width-4, "↑/↓", "Navigate", "enter", "Resume", "/", "Filter", "esc/q", "Close")

	items := v.picker.VisibleItems()
	start, end := paneWindow(len(items), v.picker.Index(), maxSessionResumeRows, layoutModeForHeight(ctx.height))
	listRows := make([]string, 0, end-start+1)
	if v.picker.SettingFilter() || v.picker.IsFiltered() {
		listRows = append(listRows, mutedStyle.Render("Search: ")+userStyle.Render(v.picker.FilterValue()))
	}
	contentWidth := maxInt(20, ctx.width-8)
	for index := start; index < end; index++ {
		item, ok := items[index].(sessionListItem)
		if !ok {
			continue
		}
		prefix, style := "  ", bodyStyle
		if index == v.picker.Index() {
			prefix, style = "> ", brandStyle
		}
		idLabel := item.summary.ID
		if item.isCurrent {
			idLabel += " [current]"
		}
		updated := item.summary.UpdatedAt.Local().Format("01/02 15:04")
		profile := item.summary.AgentProfile
		if profile == "" {
			profile = "universal"
		}
		meta := fmt.Sprintf("%s · %s", updated, profile)
		if item.summary.MessageCount > 0 {
			meta = fmt.Sprintf("%s · %d msgs · %s", updated, item.summary.MessageCount, profile)
		}
		metaWidth := len([]rune(meta))
		rem := contentWidth - metaWidth - 4
		if rem < 10 {
			rem = 10
		}
		idFormatted := truncateWithEllipsis(idLabel, rem)
		gap := maxInt(2, contentWidth-len([]rune(idFormatted))-metaWidth-2)
		line := prefix + style.Render(idFormatted) + strings.Repeat(" ", gap) + mutedStyle.Render(meta)
		listRows = append(listRows, line)
	}

	status := fmt.Sprintf("%d sessions", len(v.items))
	if selected, ok := v.selectedItem(); ok {
		preview := strings.TrimSpace(selected.summary.Preview)
		if preview != "" {
			status = selected.summary.ID + " · " + truncateWithEllipsis(preview, 40)
		} else {
			status = selected.summary.ID
		}
	}
	rows := paneSection("Sessions", listRows, help, status, ctx.width)
	return renderModalRows(ctx, accentAssistant, rows)
}

func (v *sessionResumePaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	if !v.initialized || len(v.items) == 0 {
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: sessionResumeViewID}}
	}
	switch {
	case key.Matches(message, paneutil.Keys.Close):
		if !v.picker.SettingFilter() && !v.picker.IsFiltered() {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: sessionResumeViewID}}
		}
	case key.Matches(message, paneutil.Keys.Confirm):
		if !v.picker.SettingFilter() {
			if selected, ok := v.selectedItem(); ok {
				return paneKeyResult{handled: true, action: paneAction{kind: paneActionResumeSession, sessionID: selected.summary.ID}}
			}
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: sessionResumeViewID}}
		}
	case key.Matches(message, paneutil.Keys.Escape):
		if !v.picker.SettingFilter() && !v.picker.IsFiltered() {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: sessionResumeViewID}}
		}
	case message.Text == "q":
		if !v.picker.SettingFilter() {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: sessionResumeViewID}}
		}
	}

	updated, cmd := v.picker.Update(message)
	v.picker = updated
	return paneKeyResult{handled: true, cmd: cmd}
}

func (m *bubbleModel) openSessionResumePane() {
	if m == nil || m.sessions == nil {
		m.appendError("session service is unavailable")
		return
	}
	if m.panes.bottom.has(sessionResumeViewID) {
		m.panes.bottom.remove(sessionResumeViewID)
		m.requestRelayout()
		return
	}
	summaries, err := m.sessions.ListSummaries(m.ctx, app.SessionListOptions{
		Limit: 0,
	})
	if err != nil {
		m.appendError("failed to list sessions: " + err.Error())
		return
	}
	if len(summaries) == 0 {
		m.appendMuted("no sessions found")
		return
	}

	items := make([]sessionListItem, 0, len(summaries))
	initialIndex := 0
	for i, s := range summaries {
		isCurrent := s.ID == m.sessionID
		if isCurrent && initialIndex == 0 {
			initialIndex = i
		}
		items = append(items, sessionListItem{
			summary:   s,
			isCurrent: isCurrent,
		})
	}

	listItems := make([]list.Item, 0, len(items))
	for _, item := range items {
		listItems = append(listItems, item)
	}

	picker := paneutil.NewMinimalList(listItems, sessionResumeDelegate{}, defaultBubbleWidth-8, 7)
	picker.InfiniteScrolling = true
	picker.Select(initialIndex)

	m.panes.bottom.push(&sessionResumePaneView{
		picker:      picker,
		items:       items,
		initialized: true,
	})
	m.requestRelayout()
}
