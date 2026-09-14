package runtime

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
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
	input         textarea.Model
	history       []string
	historyPos    int
	draft         string
	historyDrafts map[int]string
	bashMode      bool
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
	icons    tuistyle.IconSet
}

func newBottomPane(hasRunner bool, reducedMotion bool) *bottomPane {
	input := newPrompt(hasRunner, reducedMotion)
	return &bottomPane{
		composer: composerState{input: input, history: make([]string, 0), historyPos: 0},
		views:    make([]bottomPaneView, 0),
		icons:    tuistyle.UnicodeIcons,
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
	id := view.ID()
	if id != "" {
		p.remove(id)
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

func (p *bottomPane) setIcons(icons tuistyle.IconSet) {
	if p == nil {
		return
	}
	p.icons = tuistyle.OrUnicodeIcons(icons)
	applyPromptChrome(&p.composer.input, p.composer.bashMode, p.icons)
}

func (p *bottomPane) setBashMode(on bool) {
	if p == nil {
		return
	}
	p.composer.bashMode = on
	applyPromptChrome(&p.composer.input, on, p.icons)
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
	p.composer.historyDrafts = nil
}

func (p *bottomPane) saveHistoryDraft() {
	if p == nil {
		return
	}
	if p.composer.historyDrafts == nil {
		p.composer.historyDrafts = make(map[int]string)
	}
	p.composer.historyDrafts[p.composer.historyPos] = p.composer.input.Value()
	if p.composer.historyPos == len(p.composer.history) {
		p.composer.draft = p.composer.input.Value()
	}
}

func (p *bottomPane) restoreHistoryDraft(pos int) {
	if p == nil {
		return
	}
	if draft, ok := p.composer.historyDrafts[pos]; ok {
		p.composer.input.SetValue(draft)
	} else if pos < len(p.composer.history) {
		p.composer.input.SetValue(p.composer.history[pos])
	} else {
		p.composer.input.SetValue(p.composer.draft)
	}
	p.composer.input.CursorEnd()
}

func (p *bottomPane) historyPrevious() {
	if p == nil || len(p.composer.history) == 0 || p.composer.historyPos == 0 {
		return
	}
	p.saveHistoryDraft()
	p.composer.historyPos--
	p.restoreHistoryDraft(p.composer.historyPos)
}

func (p *bottomPane) historyNext() {
	if p == nil || p.composer.historyPos >= len(p.composer.history) {
		return
	}
	p.saveHistoryDraft()
	p.composer.historyPos++
	p.restoreHistoryDraft(p.composer.historyPos)
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

func newPrompt(hasRunner bool, reducedMotion bool) textarea.Model {
	prompt := textarea.New()
	prompt.Placeholder = promptPlaceholder(hasRunner, permission.ModeAsk, false)
	prompt.CharLimit = 0
	prompt.DynamicHeight = true
	prompt.MinHeight = 1
	prompt.MaxHeight = 4
	prompt.ShowLineNumbers = false
	prompt.EndOfBufferCharacter = ' '
	configureComposerNewline(&prompt, nil, keyboardCapabilityUnknown)
	styles := prompt.Styles()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Blurred.CursorLine = lipgloss.NewStyle()
	// A static caret keeps the insertion point visible without self-running
	// motion; bubbles maps Blink=false to its visible non-blinking cursor mode.
	if reducedMotion {
		styles.Cursor.Blink = false
	}
	prompt.SetStyles(styles)
	applyPromptChrome(&prompt, false, tuistyle.UnicodeIcons)
	_ = prompt.Focus()
	return prompt
}

func applyPromptChrome(prompt *textarea.Model, bash bool, icons tuistyle.IconSet) {
	icons = tuistyle.OrUnicodeIcons(icons)
	prefix := icons.Composer
	accent := accentAssistant
	if bash {
		// Preserve the existing shell-mode affordance independently from the
		// terminal font profile; only the normal assistant prompt is semantic.
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
	m.panes.bottom.composer.historyPos = len(m.panes.bottom.composer.history)
	m.panes.bottom.composer.draft = ""
	m.panes.bottom.composer.historyDrafts = nil
	m.requestRelayout()
}

func (m *bubbleModel) normalizeBlankComposer() bool {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return false
	}
	prompt := m.panes.bottom.prompt()
	if prompt.Value() == "" || strings.TrimSpace(prompt.Value()) != "" {
		return false
	}
	m.resetPrompt()
	return true
}

func (m *bubbleModel) historyPrevious() {
	m.panes.bottom.historyPrevious()
}

func (m *bubbleModel) historyNext() {
	m.panes.bottom.historyNext()
}

func normalizePastedPath(content string, workDir string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}
	if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, "\r") {
		return content
	}

	cleanCandidate := func(cand string) string {
		cand = strings.TrimSpace(cand)
		if cand == "" {
			return ""
		}
		if (strings.HasPrefix(cand, "'") && strings.HasSuffix(cand, "'") && len(cand) >= 2) ||
			(strings.HasPrefix(cand, "\"") && strings.HasSuffix(cand, "\"") && len(cand) >= 2) {
			cand = cand[1 : len(cand)-1]
		}
		if strings.HasPrefix(cand, "file://") {
			cand = strings.TrimPrefix(cand, "file://")
			if unescaped, err := url.PathUnescape(cand); err == nil {
				cand = unescaped
			}
		}
		if strings.Contains(cand, `\ `) {
			cand = strings.ReplaceAll(cand, `\ `, " ")
		}
		cand = filepath.Clean(cand)
		if _, err := os.Stat(cand); err == nil {
			if workDir != "" {
				if rel, err := filepath.Rel(workDir, cand); err == nil && !strings.HasPrefix(rel, "..") {
					return rel
				}
				if evalWorkDir, err := filepath.EvalSymlinks(workDir); err == nil {
					if evalCand, err := filepath.EvalSymlinks(cand); err == nil {
						if rel, err := filepath.Rel(evalWorkDir, evalCand); err == nil && !strings.HasPrefix(rel, "..") {
							return rel
						}
					}
				}
			}
			return cand
		}
		return ""
	}

	if cleaned := cleanCandidate(trimmed); cleaned != "" {
		return cleaned
	}
	return content
}
