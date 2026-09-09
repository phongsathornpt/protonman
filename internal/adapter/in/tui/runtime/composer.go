package runtime

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

type bottomPaneView interface {
	ID() string
	Render(*bubbleModel) string
	HandleKey(*bubbleModel, tea.KeyPressMsg) (handled bool, cmd tea.Cmd)
	ReplacesComposer() bool
} // bottomPaneView is a transient interaction surface that can replace or augment
// the composer. Permission prompts and slash completion are the first users;
// pickers and MCP elicitation can implement the same contract later.

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
	return &bottomPane{composer: composerState{input: input, history: make([]string, 0), historyPos: 0}, views: make([]bottomPaneView, 0)}
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
	p.composer.input.Placeholder = promptPlaceholder(hasRunner, permission.ModeAsk, false)
}

func (p *bottomPane) setPlaceholder(text string) {
	if p == nil {
		return
	}
	p.composer.input.Placeholder = text
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
		newLen := len(p.composer.history) - overflow
		clear(p.composer.history[newLen:])
		p.composer.history = p.composer.history[:newLen]
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

func newPrompt(hasRunner bool) textarea.Model {
	prompt := textarea.New()
	prompt.Placeholder = promptPlaceholder(hasRunner, permission.ModeAsk, false)
	prompt.CharLimit = 20_000
	prompt.ShowLineNumbers = false
	prompt.EndOfBufferCharacter = ' '
	prompt.KeyMap.InsertNewline.SetKeys("ctrl+j")
	prompt.KeyMap.InsertNewline.SetEnabled(true)
	styles := prompt.Styles()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Blurred.CursorLine = lipgloss.NewStyle()
	prompt.SetStyles(styles)
	applyPromptChrome(&prompt, false)
	_ = prompt.Focus()
	return prompt
}

func applyPromptChrome(prompt *textarea.Model, bash bool) {
	prefix := glyphPrompt
	accent := accentAssistant
	if bash {
		prefix = "! "
		accent = commandColor
	}
	prompt.Prompt = prefix
	styles := prompt.Styles()
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(accent)
	styles.Focused.Text = bodyStyle
	styles.Focused.Placeholder = mutedStyle
	styles.Blurred = styles.Focused
	styles.Blurred.CursorLine = lipgloss.NewStyle()
	prompt.SetStyles(styles)
}

func (m *bubbleModel) setBashMode(on bool) {
	m.bottom.setBashMode(on)
	m.syncSlashView()
}

func (m *bubbleModel) resetPrompt() {
	if m == nil || m.bottom == nil || m.bottom.prompt() == nil {
		return
	}
	m.bottom.prompt().Reset()
}

func (m *bubbleModel) historyPrevious() {
	m.bottom.historyPrevious()
}

func (m *bubbleModel) historyNext() {
	m.bottom.historyNext()
}
