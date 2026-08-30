package tui

import (
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
		views: make([]bottomPaneView, 0, 2),
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
	p.remove(view.ID())
	p.views = append(p.views, view)
}

func (p *bottomPane) pop() bottomPaneView {
	if p == nil || len(p.views) == 0 {
		return nil
	}
	last := len(p.views) - 1
	view := p.views[last]
	p.views = p.views[:last]
	return view
}

func (p *bottomPane) remove(id string) {
	if p == nil || id == "" {
		return
	}
	filtered := p.views[:0]
	for _, view := range p.views {
		if view.ID() != id {
			filtered = append(filtered, view)
		}
	}
	p.views = filtered
}

func (p *bottomPane) find(id string) bottomPaneView {
	if p == nil {
		return nil
	}
	for i := len(p.views) - 1; i >= 0; i-- {
		if p.views[i].ID() == id {
			return p.views[i]
		}
	}
	return nil
}

func (p *bottomPane) has(id string) bool { return p.find(id) != nil }

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
}

func (p *bottomPane) historyPrevious() {
	if p == nil || len(p.composer.history) == 0 || p.composer.historyPos == 0 {
		return
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
		p.composer.input.Reset()
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
