package runtime

import (
	"log/slog"
	"os"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
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
	slog.DebugContext(m.ctx, "tui keyboard capability changed",
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
	slog.DebugContext(m.ctx, "tui key event",
		"key", message.Keystroke(),
		"code", int64(message.Code),
		"mod", uint64(message.Mod),
		"keyboard", m.keyboardCapability.String(),
		"ssh", os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != "",
		"tmux", os.Getenv("TMUX") != "",
		"term", os.Getenv("TERM"),
		"term_program", os.Getenv("TERM_PROGRAM"),
	)
}
