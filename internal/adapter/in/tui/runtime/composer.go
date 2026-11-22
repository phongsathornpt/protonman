package runtime

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/composer"
	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
)

type panePresentationMode uint8

const (
	// paneOverlay is currently unused: no view reports it. It is retained as the
	// iota zero value so a pane that forgets to implement PresentationMode
	// degrades to the overlay position rather than silently becoming blocking.
	// Removing it would change that default, so treat its removal as a behavior
	// change rather than dead-code cleanup.
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

const (
	maxCommandHistory         = 500
	maxComposerVisibleRows    = 8
	maxComposerContentRows    = 10_000 // Keep the draft budget above the visible viewport cap.
	minTranscriptViewportRows = 5
)

type composerState struct {
	input         textarea.Model
	history       []string
	historyPos    int
	draft         string
	historyDrafts map[int]string
	bashMode      bool
	attachments   composer.State
}

type paneState struct {
	bottom              *bottomPane
	transcript          viewport.Model
	showTranscript      bool
	rawTranscript       bool
	transcriptTailOnly  bool
	transcriptTailStale bool
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

func (p *bottomPane) attachImage(path string) {
	if p == nil {
		return
	}
	p.composer.attachments.Attach(&p.composer.input, path)
}

func (p *bottomPane) setIcons(icons tuistyle.IconSet) {
	if p == nil {
		return
	}
	p.icons = tuistyle.OrUnicodeIcons(icons)
	p.syncPromptChrome()
}

func (p *bottomPane) setBashMode(on bool) {
	if p == nil {
		return
	}
	p.composer.bashMode = on
	p.syncPromptChrome()
}

func (p *bottomPane) syncPromptChrome() {
	if p == nil {
		return
	}
	applyPromptChrome(&p.composer.input, p.composer.bashMode, p.icons)
}

func (p *bottomPane) setHasRunner(hasRunner bool) {
	if p == nil {
		return
	}
	p.composer.input.Placeholder = promptPlaceholder(hasRunner)
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
	if p == nil || strings.TrimSpace(line) == "" {
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
	p.syncPromptChrome()
}

func (p *bottomPane) historyPrevious() {
	if p == nil || p.composer.attachments.Len() > 0 || len(p.composer.history) == 0 || p.composer.historyPos == 0 {
		return
	}
	p.saveHistoryDraft()
	p.composer.historyPos--
	p.restoreHistoryDraft(p.composer.historyPos)
}

func (p *bottomPane) historyNext() {
	if p == nil || p.composer.attachments.Len() > 0 || p.composer.historyPos >= len(p.composer.history) {
		return
	}
	p.saveHistoryDraft()
	p.composer.historyPos++
	p.restoreHistoryDraft(p.composer.historyPos)
}

func (p *bottomPane) historyNavigating() bool {
	return p != nil && p.composer.attachments.Len() == 0 && p.composer.historyPos < len(p.composer.history)
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
	prompt.Placeholder = promptPlaceholder(hasRunner)
	prompt.CharLimit = 0
	prompt.DynamicHeight = true
	prompt.MinHeight = 1
	prompt.MaxHeight = maxComposerVisibleRows
	prompt.MaxContentHeight = maxComposerContentRows
	prompt.ShowLineNumbers = false
	prompt.EndOfBufferCharacter = ' '
	configureComposerNewline(&prompt, nil, keyboardCapabilityUnknown)
	styles := prompt.Styles()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Blurred.CursorLine = lipgloss.NewStyle()
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
		prefix = icons.Bash
		accent = commandColor
		prompt.Placeholder = "Run workspace shell command…"
	} else {
		prompt.Placeholder = "Ask universal to build, test, or type / for commands…"
	}
	prompt.Prompt = prefix
	styles := prompt.Styles()
	styles.Focused.Prompt = lipgloss.NewStyle().Bold(true).Foreground(accent)
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
	m.panes.bottom.composer.attachments.Clear()
	m.panes.bottom.composer.historyPos = len(m.panes.bottom.composer.history)
	m.panes.bottom.composer.draft = ""
	m.panes.bottom.composer.historyDrafts = nil
	m.panes.bottom.remove(slashViewID)
	m.panes.bottom.remove(mentionViewID)
	m.panes.bottom.syncPromptChrome()
	m.requestRelayout()
}

func (m *bubbleModel) historyPrevious() {
	m.panes.bottom.historyPrevious()
}

func (m *bubbleModel) historyNext() {
	m.panes.bottom.historyNext()
}

func cleanupQueuedAttachments(input tuiconv.QueuedInput) {
	composer.CleanupQueuedAttachments(input.Attachments)
}

func (m *bubbleModel) clearQueuedInputs() {
	if m == nil || m.conversation == nil {
		return
	}
	for _, input := range m.conversation.QueuedInputs() {
		cleanupQueuedAttachments(input)
	}
	m.conversation.ClearQueue()
}

func submissionDisplayText(input tuiconv.QueuedInput) string {
	if strings.TrimSpace(input.DisplayText) != "" {
		if len(input.Attachments) == 0 {
			return strings.TrimSpace(input.DisplayText)
		}
		parts := []string{strings.TrimSpace(input.DisplayText)}
		for i, attachment := range input.Attachments {
			label := attachment.Placeholder
			if strings.TrimSpace(label) == "" {
				label = composer.ImageLabel(i + 1)
			}
			parts = append(parts, label)
		}
		return strings.Join(parts, " ")
	}
	parts := make([]string, 0, len(input.Attachments)+1)
	if strings.TrimSpace(input.Text) != "" {
		parts = append(parts, strings.TrimSpace(input.Text))
	}
	for i, attachment := range input.Attachments {
		label := attachment.Placeholder
		if strings.TrimSpace(label) == "" {
			label = composer.ImageLabel(i + 1)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " ")
}

func submissionHistoryText(input tuiconv.QueuedInput) string {
	if len(input.Attachments) > 0 {
		return strings.TrimSpace(input.Text)
	}
	return strings.TrimSpace(submissionDisplayText(input))
}

func (m *bubbleModel) currentModelAcceptsImageInput() bool {
	if m == nil || strings.TrimSpace(m.activeModel) == "" {
		return true
	}
	var remote *model.RemoteModel
	if candidate, ok := m.activeRemoteModel(); ok {
		remote = &candidate
	}
	profile := model.ResolveModelProfile(m.activeProvider, m.activeModel, remote)
	return profile.Capabilities.Vision != modelprofile.SupportNo
}

func (m *bubbleModel) imageInputsNotSupportedMessage() string {
	modelID := strings.TrimSpace(m.activeModel)
	if modelID == "" {
		modelID = "current model"
	}
	return fmt.Sprintf("model %s does not support image inputs; remove images or switch models", modelID)
}
