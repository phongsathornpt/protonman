package runtime

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/cmdpolicy"
	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
)

const keyboardDebugEnv = "PROTONMAN_DEBUG_KEYS"

func newComposerNewlineBinding(capability keyboardCapability) key.Binding {
	binding := key.NewBinding(key.WithKeys(composerNewlineKeyNames()...))
	setComposerNewlineHelp(&binding, capability)
	return binding
}

func configureComposerNewline(prompt *textarea.Model, binding *key.Binding, capability keyboardCapability) {
	if prompt != nil {
		prompt.KeyMap.InsertNewline.SetKeys(composerNewlineKeyNames()...)
		prompt.KeyMap.InsertNewline.SetEnabled(true)
	}
	if binding != nil {
		binding.SetKeys(composerNewlineKeyNames()...)
		setComposerNewlineHelp(binding, capability)
	}
}

func (c keyboardCapability) String() string {
	switch c {
	case keyboardCapabilityLegacy:
		return "legacy"
	case keyboardCapabilityDisambiguated:
		return "disambiguated"
	default:
		return "unknown"
	}
}

func (m *bubbleModel) updateKeyboardCapability(message tea.KeyboardEnhancementsMsg) {
	previous := m.keyboardCapability
	if message.SupportsKeyDisambiguation() {
		m.keyboardCapability = keyboardCapabilityDisambiguated
	} else {
		m.keyboardCapability = keyboardCapabilityLegacy
	}
	configureComposerNewline(nil, &m.keys.Newline, m.keyboardCapability)
	m.debugKeyboardCapability(previous)
}
func keyboardDebugEnabled() bool {
	return os.Getenv(keyboardDebugEnv) == "1"
}

func (m *bubbleModel) debugKeyboardCapability(previous keyboardCapability) {
	if !keyboardDebugEnabled() || previous == m.keyboardCapability {
		return
	}
	slog.InfoContext(m.ctx, "tui keyboard capability changed",
		"from", previous.String(),
		"to", m.keyboardCapability.String(),
		"ssh", os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != "",
		"tmux", os.Getenv("TMUX") != "",
		"term", os.Getenv("TERM"),
		"term_program", os.Getenv("TERM_PROGRAM"),
	)
}

func (m *bubbleModel) debugKeyPress(message tea.KeyPressMsg) {
	if !keyboardDebugEnabled() || (message.Mod == 0 && message.Text != "") {
		return
	}
	slog.InfoContext(m.ctx, "tui key event",
		"key", message.Keystroke(),
		"action", m.composerAction(message),
		"code", int64(message.Code),
		"mod", uint64(message.Mod),
		"keyboard", m.keyboardCapability.String(),
		"ssh", os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != "",
		"tmux", os.Getenv("TMUX") != "",
		"term", os.Getenv("TERM"),
		"term_program", os.Getenv("TERM_PROGRAM"),
	)
}

const (
	maxQueuedPrompts     = 32
	maxQueuePreviewRunes = 160
)

func (m *bubbleModel) composerInput() tuiconv.QueuedInput {
	if m == nil || m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return tuiconv.QueuedInput{}
	}
	prompt := m.panes.bottom.prompt()
	attachments := m.panes.bottom.composer.attachments.snapshot(prompt)
	return tuiconv.QueuedInput{
		Text:        stripAttachmentPlaceholders(prompt.Value(), attachments),
		Attachments: attachments,
	}
}

func (m *bubbleModel) submit() tea.Cmd {
	prompt := m.panes.bottom.prompt()
	line := strings.TrimSpace(prompt.Value())
	if m.panes.bottom.bashMode() {
		if line == "" {
			m.resetPrompt()
			m.panes.bottom.remove(slashViewID)
			m.setBashMode(false)
			return nil
		}
		if m.busy || m.imagePreparing || m.hasPermissionView() {
			if !m.enqueuePrompt("!" + line) {
				return nil
			}
			m.resetPrompt()
			m.panes.bottom.remove(slashViewID)
			m.refreshViewport()
			return nil
		}
		m.resetPrompt()
		m.panes.bottom.remove(slashViewID)
		m.setBashMode(false)
		return m.dispatchBang(line)
	}

	input := m.composerInput()
	if strings.TrimSpace(input.Text) == "" && len(input.Attachments) == 0 {
		if prompt.Value() != "" {
			m.resetPrompt()
		}
		return nil
	}
	if len(input.Attachments) == 0 && strings.HasPrefix(input.Text, "/") && isCommandLine(input.Text) {
		m.resetPrompt()
		m.panes.bottom.remove(slashViewID)
		return m.dispatch(input.Text)
	}
	if len(input.Attachments) > 0 && !m.currentModelAcceptsImageInput() {
		m.appendError(m.imageInputsNotSupportedMessage())
		m.refreshViewport()
		return nil
	}
	if m.busy || m.imagePreparing || m.hasPermissionView() {
		if !m.enqueueInput(input) {
			return nil
		}
		m.resetPrompt()
		m.panes.bottom.remove(slashViewID)
		m.refreshViewport()
		return nil
	}
	m.resetPrompt()
	m.panes.bottom.remove(slashViewID)
	return m.dispatchInput(input)
}

func (m *bubbleModel) enqueuePrompt(line string) bool {
	return m.enqueueInput(tuiconv.QueuedInput{Text: line})
}

func (m *bubbleModel) enqueueInput(input tuiconv.QueuedInput) bool {
	if m.conversation == nil || !m.conversation.EnqueueInput(input) {
		m.appendMuted(fmt.Sprintf("queue full (%d); finish or cancel the active turn before adding more", tuiconv.DefaultMaxQueuedPrompts))
		m.refreshViewport()
		return false
	}
	preview := submissionDisplayText(input)
	m.appendMuted(fmt.Sprintf("queued (%d): %s", m.conversation.QueueLen(), tuiconv.QueuePreview(preview, maxQueuePreviewRunes)))
	return true
}

func (m *bubbleModel) drainQueue() tea.Cmd {
	if m.busy || m.imagePreparing || m.hasPermissionView() || m.conversation == nil || m.conversation.QueueLen() == 0 {
		return nil
	}
	input, ok := m.conversation.DequeueInput()
	if !ok {
		return nil
	}
	if len(input.Attachments) == 0 && strings.HasPrefix(input.Text, "!") && !isCommandLine(input.Text) {
		return m.dispatchBang(strings.TrimPrefix(input.Text, "!"))
	}
	return m.dispatchInput(input)
}

func (m *bubbleModel) dispatch(line string) tea.Cmd {
	return m.dispatchInput(tuiconv.QueuedInput{Text: line})
}

func (m *bubbleModel) dispatchInput(input tuiconv.QueuedInput) tea.Cmd {
	display := submissionDisplayText(input)
	if history := submissionHistoryText(input); history != "" {
		m.panes.bottom.recordHistory(history)
	}
	if len(input.Attachments) == 0 && isCommandLine(input.Text) {
		parsed := parseCommand(input.Text)
		if spec, ok := slashview.LookupCommand(parsed.Name); ok && spec.EchoUser {
			m.appendUser(input.Text)
		}
		if m.rejectBlockedSlashCommand(input.Text) {
			return nil
		}
		return m.executeCommand(input.Text)
	}
	if len(input.Attachments) > 0 && !m.currentModelAcceptsImageInput() {
		m.appendError(m.imageInputsNotSupportedMessage())
		m.restoreSubmissionToComposer(input)
		return nil
	}
	m.appendUser(display)
	if len(input.Attachments) > 0 {
		m.imagePreparing = true
		m.activity = "preparing image"
		m.requestRelayout()
		return prepareImageSubmission(input)
	}
	return m.startTurn(input.Text)
}

func (m *bubbleModel) dispatchBang(command string) tea.Cmd {
	slog.DebugContext(m.ctx, "tui direct bash submitted", "command_bytes", len(command))
	m.panes.bottom.recordHistory("!" + command)
	m.appendUser("!" + command)
	return m.startBash(command)
}

func (m *bubbleModel) rejectBlockedSlashCommand(line string) bool {
	if m == nil || !strings.HasPrefix(strings.TrimSpace(line), "/") {
		return false
	}
	if !m.busy && !m.hasPermissionView() {
		return false
	}

	cmd := cmdpolicy.Classify(line)
	if !slashCommandRequiresIdle(cmd) {
		return false
	}

	m.appendError(blockedSlashCommandMessage(cmd, m.busy))
	m.refreshViewport()
	return true
}

func slashCommandRequiresIdle(cmd cmdpolicy.Command) bool {
	switch cmd.Kind {
	case cmdpolicy.KindHelp, cmdpolicy.KindTodo, cmdpolicy.KindAgents, cmdpolicy.KindQuit, cmdpolicy.KindUnknown:
		return false
	case cmdpolicy.KindGoal:
		return strings.TrimSpace(cmd.Rest) != ""
	case cmdpolicy.KindSkills:
		arg := strings.ToLower(strings.TrimSpace(cmd.Argument))
		return arg != "active" && arg != "check" && arg != "verify"
	case cmdpolicy.KindProvider:
		return !strings.EqualFold(strings.TrimSpace(cmd.Argument), "list")
	case cmdpolicy.KindPermission, cmdpolicy.KindLow, cmdpolicy.KindClear, cmdpolicy.KindResume, cmdpolicy.KindModel, cmdpolicy.KindCall:
		return true
	default:
		return true
	}
}

func blockedSlashCommandMessage(cmd cmdpolicy.Command, busy bool) string {
	if !busy {
		return fmt.Sprintf("cannot run /%s while a permission request is active", cmd.Name)
	}
	switch cmd.Kind {
	case cmdpolicy.KindPermission:
		return "cannot change permission mode while a turn is running"
	case cmdpolicy.KindLow:
		return "cannot change low concurrency mode while a turn is running"
	case cmdpolicy.KindSkills:
		return "cannot change skills while a turn is running"
	case cmdpolicy.KindGoal:
		return "cannot change goal while a turn is running"
	case cmdpolicy.KindClear:
		return "cannot clear conversation while a turn is running"
	case cmdpolicy.KindResume:
		return "cannot switch session while a turn is running"
	case cmdpolicy.KindModel:
		return "cannot change model while a turn is running"
	case cmdpolicy.KindProvider:
		return "cannot change provider while a turn is running"
	case cmdpolicy.KindCall:
		return "cannot start a direct tool call while a turn is running"
	default:
		return fmt.Sprintf("cannot run /%s while a turn is running", cmd.Name)
	}
}
