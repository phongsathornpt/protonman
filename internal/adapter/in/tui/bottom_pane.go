package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
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

const maxCommandHistory = 500

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

func (p *bottomPane) setHasRunner(hasRunner bool) {
	if p == nil {
		return
	}
	p.composer.input.Placeholder = promptPlaceholder(hasRunner)
}

func (p *bottomPane) bashMode() bool {
	return p != nil && p.composer.bashMode
}

func (p *bottomPane) recordHistory(line string) {
	if p == nil {
		return
	}
	p.composer.history = append(p.composer.history, line)
	if overflow := len(p.composer.history) - maxCommandHistory; overflow > 0 {
		copy(p.composer.history, p.composer.history[overflow:])
		p.composer.history = p.composer.history[:maxCommandHistory]
	}
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
		return renderModalRows(m, accentAssistant, []string{"No agent skills discovered.", "", fmt.Sprintf("Place skills in %s or .proton/skills/.", appdirs.UserSkillsDisplay()), "", "esc close"})
	}
	skills := m.skills.List()
	visibleRows := pickerVisibleRows(m.height, maxSkillsRows)
	index, offset, visibleEnd := normalizedPickerWindow(v.index, v.offset, len(skills), visibleRows)
	visible := skills[offset:visibleEnd]

	maxWidth := maxInt(1, m.width-8)
	rows := make([]string, 0, len(visible)+6)
	activeCount := len(m.skills.ActivatedList())
	title := fmt.Sprintf("Agent Skills (%d/%d active · item %d of %d)", activeCount, len(skills), index+1, len(skills))
	rows = append(rows, brandStyle.Render(title), "")

	if offset > 0 {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ▲ %d more above", offset)))
	}

	for i, s := range visible {
		idx := offset + i
		isCurrent := idx == index
		isActive := m.skills.IsActivated(s.Name)

		cursor := "  "
		if isCurrent {
			cursor = glyphPrompt
		}

		box := mutedStyle.Render("[ ]")
		if isActive {
			box = successStyle.Render("[x]")
		}

		nameStr := s.Name
		if isCurrent {
			nameStr = brandStyle.Bold(true).Render(s.Name)
		} else if isActive {
			nameStr = assistantStyle.Bold(true).Render(s.Name)
		} else {
			nameStr = assistantStyle.Render(s.Name)
		}

		line := fmt.Sprintf("%s%s %s", cursor, box, nameStr)
		rows = append(rows, wrapWords(line, maxWidth))
	}

	if visibleEnd < len(skills) {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("  ▼ %d more below", len(skills)-visibleEnd)))
	}

	footer := "j/k move · space toggle · pgup/pgdn · esc/enter close"
	if layoutModeForHeight(m.height) == layoutTiny {
		footer = "↑/↓ · space · esc"
		rows = compactPickerRows(rows)
	}
	rows = append(rows, "", mutedStyle.Render(footer))
	return renderModalRows(m, accentAssistant, rows)
}

func (v *skillsPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	defer func() {
		if m != nil && m.skills != nil {
			visible := pickerVisibleRows(m.height, maxSkillsRows)
			v.index, v.offset, _ = normalizedPickerWindow(v.index, v.offset, len(m.skills.List()), visible)
		}
	}()
	if key.Matches(message, m.keys.ToggleSkills) {
		m.bottom.remove(skillsViewID)
		return true, nil
	}
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
	case " ", "t":
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
	case "esc", "enter", "q":
		m.bottom.remove(skillsViewID)
		return true, nil
	default:
		return false, nil
	}
}
