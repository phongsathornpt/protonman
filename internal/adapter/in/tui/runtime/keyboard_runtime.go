package runtime

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
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
		if m.busy || m.hasPermissionView() {
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
	if line == "" {
		if prompt.Value() != "" {
			m.resetPrompt()
		}
		return nil
	}
	if m.busy || m.hasPermissionView() {
		if !m.enqueuePrompt(line) {
			return nil
		}
		m.resetPrompt()
		m.panes.bottom.remove(slashViewID)
		m.refreshViewport()
		return nil
	}
	m.resetPrompt()
	m.panes.bottom.remove(slashViewID)
	return m.dispatch(line)
}

func (m *bubbleModel) enqueuePrompt(line string) bool {
	if m.conversation == nil || !m.conversation.Enqueue(line) {
		m.appendMuted(fmt.Sprintf("queue full (%d); finish or cancel the active turn before adding more", tuiconv.DefaultMaxQueuedPrompts))
		m.refreshViewport()
		return false
	}
	m.appendMuted(fmt.Sprintf("queued (%d): %s", m.conversation.QueueLen(), tuiconv.QueuePreview(line, maxQueuePreviewRunes)))
	return true
}

func (m *bubbleModel) drainQueue() tea.Cmd {
	if m.busy || m.hasPermissionView() || m.conversation == nil || m.conversation.QueueLen() == 0 {
		return nil
	}
	line, ok := m.conversation.Dequeue()
	if !ok {
		return nil
	}
	if strings.HasPrefix(line, "!") && !isCommandLine(line) {
		return m.dispatchBang(strings.TrimPrefix(line, "!"))
	}
	return m.dispatch(line)
}

func (m *bubbleModel) dispatch(line string) tea.Cmd {
	m.panes.bottom.recordHistory(line)
	if isCommandLine(line) {
		parsed := parseCommand(line)
		if spec, ok := slashview.LookupCommand(parsed.Name); ok && spec.EchoUser {
			m.appendUser(line)
		}
		return m.executeCommand(line)
	}
	m.appendUser(line)
	return m.startTurn(line)
}

func (m *bubbleModel) dispatchBang(command string) tea.Cmd {
	slog.DebugContext(m.ctx, "tui direct bash submitted", "command_bytes", len(command))
	m.panes.bottom.recordHistory("!" + command)
	m.appendUser("!" + command)
	return m.startBash(command)
}
