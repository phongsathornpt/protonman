package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

func newPrompt(hasRunner bool) textarea.Model {
	prompt := textarea.New()
	prompt.Placeholder = promptPlaceholder(hasRunner)
	prompt.CharLimit = 20_000
	prompt.ShowLineNumbers = false
	prompt.EndOfBufferCharacter = ' '
	// Official Charm chat example: Enter sends, it does not insert a newline.
	prompt.KeyMap.InsertNewline.SetEnabled(false)
	// Drop the inverted cursor-line so the composer stays flush with the chrome.
	prompt.FocusedStyle.CursorLine = lipgloss.NewStyle()
	prompt.BlurredStyle.CursorLine = lipgloss.NewStyle()
	applyPromptChrome(&prompt, false)
	// Focus on the stored model. Init is no longer the place to do this:
	// value-receiver Init focused a copy and the live textarea dropped keys.
	_ = prompt.Focus()
	return prompt
}

func applyPromptChrome(prompt *textarea.Model, bash bool) {
	prefix := glyphPrompt
	accent := accentAssistant
	text := textPrimary
	if bash {
		prefix = "! "
		accent = commandColor
	}
	prompt.Prompt = prefix
	prompt.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(accent)
	prompt.FocusedStyle.Text = lipgloss.NewStyle().Foreground(text)
	prompt.BlurredStyle = prompt.FocusedStyle
	prompt.BlurredStyle.CursorLine = lipgloss.NewStyle()
}

func (m *bubbleModel) setBashMode(on bool) {
	m.bashMode = on
	applyPromptChrome(&m.prompt, on)
}

func (m *bubbleModel) historyPrevious() {
	if len(m.history) == 0 || m.historyPos == 0 {
		return
	}
	m.historyPos--
	m.prompt.SetValue(m.history[m.historyPos])
	m.prompt.CursorEnd()
}

func (m *bubbleModel) historyNext() {
	if m.historyPos >= len(m.history) {
		return
	}
	m.historyPos++
	if m.historyPos == len(m.history) {
		m.prompt.Reset()
		return
	}
	m.prompt.SetValue(m.history[m.historyPos])
	m.prompt.CursorEnd()
}
