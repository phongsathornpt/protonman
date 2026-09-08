package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/slashview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

const maxSlashRows = 6
const slashViewID = "slash"

type slashCommand = slashview.Command

var slashCatalog = slashview.Catalog(agent.ProfileList("|"))

type slashPaneView struct{ index int }

func (*slashPaneView) ID() string             { return slashViewID }
func (*slashPaneView) ReplacesComposer() bool { return false }
func (v *slashPaneView) Render(m *bubbleModel) string {
	return m.renderSlash(v.index)
}
func (v *slashPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	switch message.String() {
	case "up":
		m.moveSlash(-1)
		return true, nil
	case "down":
		m.moveSlash(1)
		return true, nil
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

func isCommandLine(line string) bool { return slashview.IsCommandLine(line) }
func splitCommand(line string) (string, string, []string) {
	return slashview.SplitCommand(line)
}
func canonicalSlashName(name string) string   { return slashview.CanonicalName(slashCatalog, name) }
func fuzzyContains(target, query string) bool { return slashview.FuzzyContains(target, query) }

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
		items = append(items, slashview.Skill{
			Name:        skill.Name,
			Description: skill.Description,
			Scope:       string(skill.Scope),
			Active:      m.skills.IsActivated(skill.Name),
		})
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
	view.index = max(0, min(view.index, len(matches)-1))
}

func (m bubbleModel) slashOpen() bool { return len(m.slashMatches()) > 0 }

func (m *bubbleModel) clampSlashIndex() {
	m.syncSlashView()
	view := m.slashState()
	if view == nil {
		return
	}
	matches := m.slashMatches()
	if len(matches) == 0 {
		return
	}
	view.index = max(0, min(view.index, len(matches)-1))
}

func (m *bubbleModel) moveSlash(delta int) {
	m.syncSlashView()
	view := m.slashState()
	matches := m.slashMatches()
	if view == nil || len(matches) == 0 {
		return
	}
	view.index = (view.index + delta) % len(matches)
	if view.index < 0 {
		view.index += len(matches)
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
	selected := matches[view.index]
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
	prompt.Reset()
	m.bottom.remove(slashViewID)
	return true, m.dispatch(insertion)
}

func truncateWithEllipsis(s string, maxLen int) string { return tool.TruncateRunes(s, maxLen) }

func (m bubbleModel) renderSlash(index int) string {
	matches := m.slashMatches()
	if len(matches) == 0 {
		return ""
	}
	context, _ := m.parseSlashContext()
	return slashview.Render(matches, index, m.width, maxSlashRows, context.Kind)
}

func (m bubbleModel) slashView() string {
	if view := m.slashState(); view != nil {
		return view.Render(&m)
	}
	return ""
}
