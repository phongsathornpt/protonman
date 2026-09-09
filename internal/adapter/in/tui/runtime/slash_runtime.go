package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
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
	index   int
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
		v.index = 0
		return
	}
	v.index = maxInt(0, minInt(v.index, len(matches)-1))
	v.picker.Select(v.index)
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
		v.index = v.picker.Index()
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

func isCommandLine(line string) bool {
	return slashview.IsCommandLine(line)
}

func splitCommand(line string) (string, string, []string) {
	return slashview.SplitCommand(line)
}

func canonicalSlashName(name string) string {
	return slashview.CanonicalName(slashCatalog, name)
}

func fuzzyContains(target, query string) bool {
	return slashview.FuzzyContains(target, query)
}

type slashContext = slashview.Context

const (
	slashKindCommand = slashview.ContextCommand
	slashKindSkill   = slashview.ContextSkill
)

func (m bubbleModel) parseSlashContext() (slashContext, bool) {
	if m.bottom == nil || m.bottom.bashMode() || m.bottom.has(permissionViewID) || m.bottom.has(skillsViewID) {
		return slashContext{}, false
	}
	prompt := m.bottom.prompt()
	if prompt == nil {
		return slashContext{}, false
	}
	return slashview.ParseContext(prompt.Value())
}

func (m bubbleModel) slashQuery() (prefix string, query string, ok bool) {
	context, ok := m.parseSlashContext()
	if !ok {
		return "", "", false
	}
	return context.Prefix, context.Query, true
}

func (m bubbleModel) slashMatches() []slashCommand {
	context, ok := m.parseSlashContext()
	if !ok {
		return nil
	}
	if context.Kind != slashKindSkill {
		return slashview.Matches(context, slashCatalog, nil)
	}
	if m.skills == nil {
		return nil
	}
	skills := m.skills.List()
	items := make([]slashview.Skill, 0, len(skills))
	for _, skill := range skills {
		items = append(items, slashview.Skill{Name: skill.Name, Description: skill.Description, Scope: string(skill.Scope), Active: m.skills.IsActivated(skill.Name)})
	}
	return slashview.Matches(context, slashCatalog, items)
}

func (m *bubbleModel) slashState() *slashPaneView {
	if m.bottom == nil {
		return nil
	}
	view, _ := m.bottom.find(slashViewID).(*slashPaneView)
	return view
}

func (m *bubbleModel) syncSlashView() {
	if m.bottom == nil {
		return
	}
	matches := m.slashMatches()
	if len(matches) == 0 {
		m.bottom.remove(slashViewID)
		return
	}
	view := m.slashState()
	if view == nil {
		view = &slashPaneView{}
		m.bottom.push(view)
	}
	view.sync(m)
}

func (m bubbleModel) slashOpen() bool {
	return len(m.slashMatches()) > 0
}

func (m *bubbleModel) clampSlashIndex() {
	m.syncSlashView()
	if view := m.slashState(); view != nil {
		view.sync(m)
	}
}

func (m *bubbleModel) moveSlash(delta int) {
	m.syncSlashView()
	view := m.slashState()
	if view == nil || len(view.matches) == 0 {
		return
	}
	if delta < 0 {
		view.picker.CursorUp()
	} else if delta > 0 {
		view.picker.CursorDown()
	}
	view.index = view.picker.Index()
}

func (m *bubbleModel) acceptSlash(run bool) (applied bool, command tea.Cmd) {
	m.syncSlashView()
	view := m.slashState()
	matches := m.slashMatches()
	if view == nil || len(matches) == 0 {
		return false, nil
	}
	m.clampSlashIndex()
	view = m.slashState()
	if view == nil || len(view.matches) == 0 {
		return false, nil
	}
	view.index = view.picker.Index()
	selected := view.matches[view.index]
	context, _ := m.parseSlashContext()
	prompt := m.bottom.prompt()
	var insertion string
	if context.Kind == slashKindSkill {
		insertion = context.Lead + selected.Name
	} else {
		insertion = context.Prefix + selected.Name
		if selected.TakesArgs {
			insertion += " "
			prompt.SetValue(insertion)
			prompt.CursorEnd()
			m.bottom.remove(slashViewID)
			return true, nil
		}
	}
	if !run {
		prompt.SetValue(insertion)
		prompt.CursorEnd()
		m.bottom.remove(slashViewID)
		return true, nil
	}
	m.resetPrompt()
	m.bottom.remove(slashViewID)
	return true, m.dispatch(insertion)
}

func truncateWithEllipsis(s string, maxLen int) string {
	return textview.TruncateEllipsis(s, maxLen)
}

func (m bubbleModel) renderSlash(index int) string {
	view := &slashPaneView{index: index}
	return view.Render(&m)
}

func (m bubbleModel) slashView() string {
	if view := m.slashState(); view != nil {
		return view.Render(&m)
	}
	return ""
}
