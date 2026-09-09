package runtime

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
)

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
	if len(m.queue) >= maxQueuedPrompts {
		m.appendMuted(fmt.Sprintf("queue full (%d); finish or cancel the active turn before adding more", maxQueuedPrompts))
		m.refreshViewport()
		return false
	}
	m.queue = append(m.queue, line)
	m.appendMuted(fmt.Sprintf("queued (%d): %s", len(m.queue), queuePreview(line)))
	return true
}

func queuePreview(line string) string {
	runes := []rune(strings.TrimSpace(line))
	if len(runes) <= maxQueuePreviewRunes {
		return string(runes)
	}
	return string(runes[:maxQueuePreviewRunes-1]) + "…"
}

func (m *bubbleModel) drainQueue() tea.Cmd {
	if m.busy || m.hasPermissionView() || len(m.queue) == 0 {
		return nil
	}
	line := m.queue[0]
	m.queue[0] = ""
	if len(m.queue) == 1 {
		m.queue = nil
	} else {
		m.queue = m.queue[1:]
	}
	if strings.HasPrefix(line, "!") && !isCommandLine(line) {
		return m.dispatchBang(strings.TrimPrefix(line, "!"))
	}
	return m.dispatch(line)
}

func (m *bubbleModel) dispatch(line string) tea.Cmd {
	m.panes.bottom.recordHistory(line)
	if isCommandLine(line) {
		name, _, _ := splitCommand(line)
		if name != "clear" && name != "new" && name != "quit" && name != "exit" {
			m.appendUser(line)
		}
		return m.executeCommand(line)
	}
	m.showWelcome = false
	m.appendUser(line)
	return m.startTurn(line)
}

func (m *bubbleModel) dispatchBang(command string) tea.Cmd {
	slog.DebugContext(m.ctx, "tui direct bash submitted", "command_bytes", len(command))
	m.panes.bottom.recordHistory("!" + command)
	m.showWelcome = false
	m.appendUser("!" + command)
	return m.startBash(command)
}
