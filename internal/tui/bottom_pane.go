package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// bottomPaneView is a transient interaction surface that can replace or augment
// the composer. Permission prompts and slash completion are the first users;
// pickers and MCP elicitation can implement the same contract later.
type bottomPaneView interface {
	ID() string
	Render(*bubbleModel) string
	HandleKey(*bubbleModel, tea.KeyMsg) (handled bool, cmd tea.Cmd)
	ReplacesComposer() bool
}

type composerState struct {
	input      textarea.Model
	history    []string
	historyPos int
	draft      string
	bashMode   bool
}

type bottomPane struct {
	composer composerState
	views    []bottomPaneView
}

func newBottomPane(hasRunner bool) *bottomPane {
	input := newPrompt(hasRunner)
	return &bottomPane{
		composer: composerState{
			input:      input,
			history:    make([]string, 0),
			historyPos: 0,
		},
		views: make([]bottomPaneView, 0),
	}
}

func (p *bottomPane) top() bottomPaneView {
	if p == nil || len(p.views) == 0 {
		return nil
	}
	return p.views[len(p.views)-1]
}

func (p *bottomPane) push(view bottomPaneView) {
	if p == nil || view == nil {
		return
	}
	p.views = append(p.views, view)
}

func (p *bottomPane) remove(id string) {
	if p == nil || len(p.views) == 0 {
		return
	}
	next := make([]bottomPaneView, 0, len(p.views))
	for _, v := range p.views {
		if v.ID() != id {
			next = append(next, v)
		}
	}
	p.views = next
}

func (p *bottomPane) has(id string) bool {
	if p == nil {
		return false
	}
	for _, v := range p.views {
		if v.ID() == id {
			return true
		}
	}
	return false
}

func (p *bottomPane) find(id string) bottomPaneView {
	if p == nil {
		return nil
	}
	for _, v := range p.views {
		if v.ID() == id {
			return v
		}
	}
	return nil
}

func (p *bottomPane) prompt() *textarea.Model {
	if p == nil {
		return nil
	}
	return &p.composer.input
}

func (p *bottomPane) setBashMode(on bool) {
	if p == nil {
		return
	}
	p.composer.bashMode = on
	applyPromptChrome(&p.composer.input, on)
}

func (p *bottomPane) bashMode() bool {
	return p != nil && p.composer.bashMode
}

func (p *bottomPane) recordHistory(line string) {
	if p == nil {
		return
	}
	p.composer.history = append(p.composer.history, line)
	p.composer.historyPos = len(p.composer.history)
	p.composer.draft = ""
}

func (p *bottomPane) historyPrevious() {
	if p == nil || len(p.composer.history) == 0 || p.composer.historyPos == 0 {
		return
	}
	if p.composer.historyPos == len(p.composer.history) {
		p.composer.draft = p.composer.input.Value()
	}
	p.composer.historyPos--
	p.composer.input.SetValue(p.composer.history[p.composer.historyPos])
	p.composer.input.CursorEnd()
}

func (p *bottomPane) historyNext() {
	if p == nil || p.composer.historyPos >= len(p.composer.history) {
		return
	}
	p.composer.historyPos++
	if p.composer.historyPos == len(p.composer.history) {
		p.composer.input.SetValue(p.composer.draft)
		p.composer.input.CursorEnd()
		p.composer.draft = ""
		return
	}
	p.composer.input.SetValue(p.composer.history[p.composer.historyPos])
	p.composer.input.CursorEnd()
}

func (p *bottomPane) historyNavigating() bool {
	return p != nil && p.composer.historyPos < len(p.composer.history)
}

func (p *bottomPane) renderTop(m *bubbleModel) string {
	if p == nil {
		return ""
	}
	// Keep old in-package tests that seed m.modal directly working while the
	// runtime itself owns approval state exclusively in the view stack.
	if p.top() == nil && m != nil && m.modal != nil {
		m.openPermission(*m.modal)
	}
	if top := p.top(); top != nil {
		return top.Render(m)
	}
	return ""
}

func (p *bottomPane) composerVisible() bool {
	top := p.top()
	return top == nil || !top.ReplacesComposer()
}

const skillsViewID = "skills"
const maxSkillsRows = 6

type skillsPaneView struct {
	index  int
	offset int
}

func (*skillsPaneView) ID() string             { return skillsViewID }
func (*skillsPaneView) ReplacesComposer() bool { return true }

func (v *skillsPaneView) Render(m *bubbleModel) string {
	if m == nil || m.skills == nil || len(m.skills.List()) == 0 {
		return modalStyle.
			BorderForeground(accentAssistant).
			Render("No agent skills discovered.\n\nesc close")
	}
	skills := m.skills.List()
	if v.index >= len(skills) {
		v.index = len(skills) - 1
	}
	if v.index < 0 {
		v.index = 0
	}

	// Dynamic windowing
	if v.index < v.offset {
		v.offset = v.index
	}
	if v.index >= v.offset+maxSkillsRows {
		v.offset = v.index - maxSkillsRows + 1
	}
	if v.offset > len(skills)-maxSkillsRows {
		v.offset = len(skills) - maxSkillsRows
	}
	if v.offset < 0 {
		v.offset = 0
	}

	visibleEnd := v.offset + maxSkillsRows
	if visibleEnd > len(skills) {
		visibleEnd = len(skills)
	}
	visible := skills[v.offset:visibleEnd]

	maxWidth := maxInt(1, m.width-8)
	rows := make([]string, 0, len(visible)+6)
	activeCount := len(m.skills.ActivatedList())
	title := fmt.Sprintf("Agent Skills (%d/%d active · item %d of %d)", activeCount, len(skills), v.index+1, len(skills))
	rows = append(rows, brandStyle.Render(title), "")

	if v.offset > 0 {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ▲ %d more above", v.offset)))
	}

	for i, s := range visible {
		idx := v.offset + i
		box := "[ ]"
		if m.skills.IsActivated(s.Name) {
			box = "[x]"
		}
		prefix := "  "
		line := fmt.Sprintf("%s %s [%s]: %s", box, s.Name, s.Scope, s.Description)
		if idx == v.index {
			prefix = glyphPrompt
			row := brandStyle.Render(prefix + wrapWords(line, maxWidth))
			rows = append(rows, row)
		} else {
			row := mutedStyle.Render(prefix + wrapWords(line, maxWidth))
			rows = append(rows, row)
		}
	}

	if visibleEnd < len(skills) {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ▼ %d more below", len(skills)-visibleEnd)))
	}

	rows = append(rows, "", mutedStyle.Render("j/k move · space toggle · 1-9 jump · esc/enter close"))
	return modalStyle.
		BorderForeground(accentAssistant).
		MaxWidth(maxInt(1, m.width-4)).
		Render(strings.Join(rows, "\n"))
}

func (v *skillsPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	skills := m.skills.List()
	if len(skills) == 0 {
		m.bottom.remove(skillsViewID)
		return true, nil
	}

	switch message.String() {
	case "up", "k":
		if v.index <= 0 {
			v.index = len(skills) - 1
		} else {
			v.index--
		}
		return true, nil
	case "down", "j":
		if v.index >= len(skills)-1 {
			v.index = 0
		} else {
			v.index++
		}
		return true, nil
	case "pgup":
		v.index -= 5
		if v.index < 0 {
			v.index = 0
		}
		return true, nil
	case "pgdown":
		v.index += 5
		if v.index >= len(skills) {
			v.index = len(skills) - 1
		}
		return true, nil
	case "home", "g":
		v.index = 0
		return true, nil
	case "end", "G":
		v.index = len(skills) - 1
		return true, nil
	case " ":
		if v.index >= 0 && v.index < len(skills) {
			_, _ = m.skills.Toggle(skills[v.index].Name)
		}
		return true, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		num := int(message.Runes[0] - '1')
		if num >= 0 && num < len(skills) {
			v.index = num
		}
		return true, nil
	case "esc", "enter", "q", "ctrl+s", "ctrl+c":
		m.bottom.remove(skillsViewID)
		return true, nil
	default:
		return false, nil
	}
}
