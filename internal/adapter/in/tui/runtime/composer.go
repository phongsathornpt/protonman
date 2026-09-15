package runtime

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
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

type localImageAttachment struct {
	placeholder string
	path        string
	temporary   bool
}

type attachmentState struct {
	localImages []localImageAttachment
}

type composerState struct {
	input         textarea.Model
	history       []string
	historyPos    int
	draft         string
	historyDrafts map[int]string
	bashMode      bool
	attachments   attachmentState
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

func (p *bottomPane) attachImage(path string) {
	if p == nil {
		return
	}
	p.composer.attachments.attachImage(&p.composer.input, path)
}

func (p *bottomPane) attachTemporaryImage(path string) {
	if p == nil {
		return
	}
	p.composer.attachments.attachTemporaryImage(&p.composer.input, path)
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
	if p == nil || len(p.composer.attachments.localImages) > 0 || len(p.composer.history) == 0 || p.composer.historyPos == 0 {
		return
	}
	p.saveHistoryDraft()
	p.composer.historyPos--
	p.restoreHistoryDraft(p.composer.historyPos)
}

func (p *bottomPane) historyNext() {
	if p == nil || len(p.composer.attachments.localImages) > 0 || p.composer.historyPos >= len(p.composer.history) {
		return
	}
	p.saveHistoryDraft()
	p.composer.historyPos++
	p.restoreHistoryDraft(p.composer.historyPos)
}

func (p *bottomPane) historyNavigating() bool {
	return p != nil && len(p.composer.attachments.localImages) == 0 && p.composer.historyPos < len(p.composer.history)
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
	m.panes.bottom.composer.attachments.clear()
	m.panes.bottom.composer.historyPos = len(m.panes.bottom.composer.history)
	m.panes.bottom.composer.draft = ""
	m.panes.bottom.composer.historyDrafts = nil
	m.panes.bottom.syncPromptChrome()
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

func (s *attachmentState) attachImage(prompt *textarea.Model, path string) {
	s.attachImageWithOwnership(prompt, path, false)
}

func (s *attachmentState) attachTemporaryImage(prompt *textarea.Model, path string) {
	s.attachImageWithOwnership(prompt, path, true)
}

func (s *attachmentState) attachImageWithOwnership(prompt *textarea.Model, path string, temporary bool) {
	if s == nil || prompt == nil || strings.TrimSpace(path) == "" {
		return
	}
	placeholder := localImageLabel(len(s.localImages) + 1)
	value := prompt.Value()
	if value != "" && !strings.HasSuffix(value, " ") && !strings.HasSuffix(value, "\n") {
		value += " "
	}
	value += placeholder
	prompt.SetValue(value)
	prompt.CursorEnd()
	s.localImages = append(s.localImages, localImageAttachment{placeholder: placeholder, path: path, temporary: temporary})
}

func (s *attachmentState) release() {
	if s == nil {
		return
	}
	clear(s.localImages)
	s.localImages = nil
}

func (s *attachmentState) clear() {
	s.discard()
}

func (s *attachmentState) discard() {
	if s == nil {
		return
	}
	cleanupLocalImages(s.localImages)
	s.release()
}

func cleanupLocalImages(images []localImageAttachment) {
	for _, image := range images {
		if image.temporary && strings.TrimSpace(image.path) != "" {
			_ = os.Remove(image.path)
		}
	}
}

func cleanupQueuedInputAttachments(input tuiconv.QueuedInput) {
	for _, attachment := range input.Attachments {
		if attachment.Temporary && strings.TrimSpace(attachment.Path) != "" {
			_ = os.Remove(attachment.Path)
		}
	}
}

func (m *bubbleModel) clearQueuedInputs() {
	if m == nil || m.conversation == nil {
		return
	}
	for _, input := range m.conversation.QueuedInputs() {
		cleanupQueuedInputAttachments(input)
	}
	m.conversation.ClearQueue()
}

func (s *attachmentState) syncWithText(prompt *textarea.Model) {
	if s == nil || prompt == nil || len(s.localImages) == 0 {
		return
	}
	text := prompt.Value()
	kept := make([]localImageAttachment, 0, len(s.localImages))
	for _, image := range s.localImages {
		if strings.Contains(text, image.placeholder) {
			kept = append(kept, image)
			continue
		}
		if image.temporary {
			_ = os.Remove(image.path)
		}
	}
	s.localImages = kept
	for i := range s.localImages {
		expected := localImageLabel(i + 1)
		if s.localImages[i].placeholder == expected {
			continue
		}
		text = strings.Replace(text, s.localImages[i].placeholder, expected, 1)
		s.localImages[i].placeholder = expected
	}
	if text != prompt.Value() {
		prompt.SetValue(text)
		prompt.CursorEnd()
	}
}

func (s *attachmentState) snapshot(prompt *textarea.Model) []tuiconv.Attachment {
	if s == nil {
		return nil
	}
	if prompt != nil {
		s.syncWithText(prompt)
	}
	out := make([]tuiconv.Attachment, 0, len(s.localImages))
	for _, image := range s.localImages {
		out = append(out, tuiconv.Attachment{
			Placeholder: image.placeholder,
			Path:        image.path,
			Temporary:   image.temporary,
		})
	}
	return out
}

func localImageLabel(index int) string {
	return fmt.Sprintf("[Image #%d]", index)
}

func stripAttachmentPlaceholders(text string, attachments []tuiconv.Attachment) string {
	for _, attachment := range attachments {
		if attachment.Placeholder != "" {
			text = strings.ReplaceAll(text, attachment.Placeholder, "")
		}
	}
	return strings.TrimSpace(text)
}

func submissionDisplayText(input tuiconv.QueuedInput) string {
	parts := make([]string, 0, len(input.Attachments)+1)
	if strings.TrimSpace(input.Text) != "" {
		parts = append(parts, strings.TrimSpace(input.Text))
	}
	for i, attachment := range input.Attachments {
		label := attachment.Placeholder
		if strings.TrimSpace(label) == "" {
			label = localImageLabel(i + 1)
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

func localImagePathFromPaste(content, workDir string) (string, bool) {
	if strings.TrimSpace(content) == "" || strings.ContainsAny(content, "\r\n") {
		return "", false
	}
	cleaned := normalizePastedPath(content, workDir)
	candidate := strings.TrimSpace(cleaned)
	if candidate == "" {
		return "", false
	}
	path := candidate
	if !filepath.IsAbs(path) && strings.TrimSpace(workDir) != "" {
		path = filepath.Join(workDir, path)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return filepath.Clean(path), true
	default:
		return "", false
	}
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

func (m *bubbleModel) discardPrompt() {
	if m == nil || m.panes.bottom == nil {
		return
	}
	m.panes.bottom.composer.attachments.discard()
	m.resetPrompt()
}

func (m *bubbleModel) cleanupPendingImageInput() {
	if m == nil || m.pendingImageInput == nil {
		return
	}
	cleanupQueuedInputAttachments(*m.pendingImageInput)
	m.pendingImageInput = nil
}

func attachmentPathTemporary(path string) bool {
	return strings.TrimSpace(path) != ""
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
