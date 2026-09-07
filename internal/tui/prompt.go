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
	// Enter submits. Ctrl+J inserts a newline so multi-line prompts remain
	// available without making ordinary submission ambiguous.
	prompt.KeyMap.InsertNewline.SetKeys("ctrl+j")
	prompt.KeyMap.InsertNewline.SetEnabled(true)
	prompt.FocusedStyle.CursorLine = lipgloss.NewStyle()
	prompt.BlurredStyle.CursorLine = lipgloss.NewStyle()
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
	prompt.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(accent)
	prompt.FocusedStyle.Text = bodyStyle
	prompt.BlurredStyle = prompt.FocusedStyle
	prompt.BlurredStyle.CursorLine = lipgloss.NewStyle()
}

func (m *bubbleModel) setBashMode(on bool) {
	m.bottom.setBashMode(on)
	m.syncSlashView()
}

func (m *bubbleModel) historyPrevious() { m.bottom.historyPrevious() }
func (m *bubbleModel) historyNext()     { m.bottom.historyNext() }
