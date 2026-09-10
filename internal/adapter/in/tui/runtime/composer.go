package runtime

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

type panePresentationMode uint8

const (
	paneOverlay panePresentationMode = iota
	paneBelowComposer
	paneBlocking
)

type bottomPaneView interface {
	ID() string
	Render(paneRenderContext) string
	PresentationMode() panePresentationMode
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

type paneState struct {
	bottom         *bottomPane
	transcript     viewport.Model
	showTranscript bool
	rawTranscript  bool
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
	if top := p.top(); top != nil {
		return top.Render(newPaneRenderContext(m))
	}
	return ""
}

func (p *bottomPane) composerVisible() bool {
	top := p.top()
	return top == nil || top.PresentationMode() != paneBlocking
}

func newPrompt(hasRunner bool) textarea.Model {
	prompt := textarea.New()
	prompt.Placeholder = promptPlaceholder(hasRunner, permission.ModeAsk, false)
	prompt.CharLimit = 20_000
	prompt.DynamicHeight = true
	prompt.MinHeight = 1
	prompt.MaxHeight = 4
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
	prefix := "> "
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
	m.panes.bottom.setBashMode(on)
	m.syncSlashView()
}

func (m *bubbleModel) resetPrompt() {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return
	}
	prompt := m.panes.bottom.prompt()
	prompt.Reset()
	// bubbles/textarea Reset clears the value but intentionally preserves the
	// current visual height. Collapse it so an empty composer cannot render the
	// prompt glyph once per stale multiline row.
	prompt.SetHeight(1)
	m.requestRelayout()
}

func (m *bubbleModel) historyPrevious() {
	m.panes.bottom.historyPrevious()
}

func (m *bubbleModel) historyNext() {
	m.panes.bottom.historyNext()
}
