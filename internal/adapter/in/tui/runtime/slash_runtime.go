package runtime

import (
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
)

func isCommandLine(line string) bool {
	return slashview.IsCommandLine(line)
}

func splitCommand(line string) (string, string, []string) {
	return slashview.SplitCommand(line)
}

func canonicalSlashName(name string) string {
	return slashview.CanonicalName(name)
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
	if m.panes.bottom == nil || m.panes.bottom.bashMode() || m.panes.bottom.has(permissionViewID) || m.panes.bottom.has(skillsViewID) {
		return slashContext{}, false
	}
	prompt := m.panes.bottom.prompt()
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
	if m.panes.bottom == nil {
		return nil
	}
	view, _ := m.panes.bottom.find(slashViewID).(*slashPaneView)
	return view
}

func (m *bubbleModel) syncSlashView() {
	if m.panes.bottom == nil {
		return
	}
	matches := m.slashMatches()
	if len(matches) == 0 {
		m.panes.bottom.remove(slashViewID)
		return
	}
	view := m.slashState()
	if view == nil {
		view = &slashPaneView{}
		m.panes.bottom.push(view)
	}
	view.sync(newPaneRenderContext(m))
}

func (m bubbleModel) slashOpen() bool {
	return len(m.slashMatches()) > 0
}

func (m *bubbleModel) clampSlashIndex() {
	m.syncSlashView()
	if view := m.slashState(); view != nil {
		view.sync(newPaneRenderContext(m))
	}
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
	selected := view.matches[view.picker.Index()]
	context, _ := m.parseSlashContext()
	prompt := m.panes.bottom.prompt()
	var insertion string
	if context.Kind == slashKindSkill {
		insertion = context.Lead + selected.Name
	} else {
		insertion = context.Prefix + selected.Name
		if selected.TakesArgs {
			insertion += " "
			prompt.SetValue(insertion)
			prompt.CursorEnd()
			m.panes.bottom.remove(slashViewID)
			return true, nil
		}
	}
	if !run {
		prompt.SetValue(insertion)
		prompt.CursorEnd()
		m.panes.bottom.remove(slashViewID)
		return true, nil
	}
	m.resetPrompt()
	m.panes.bottom.remove(slashViewID)
	return true, m.dispatch(insertion)
}

func truncateWithEllipsis(s string, maxLen int) string {
	return textview.TruncateEllipsis(s, maxLen)
}
