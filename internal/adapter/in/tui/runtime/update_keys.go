package runtime

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/keyboardpolicy"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"
)

var composerKeys = struct {
	ExitBash, HistoryUp, HistoryDown key.Binding
}{
	ExitBash:    key.NewBinding(key.WithKeys("backspace", "ctrl+h", "delete")),
	HistoryUp:   key.NewBinding(key.WithKeys("up")),
	HistoryDown: key.NewBinding(key.WithKeys("down")),
}

func (m *bubbleModel) semanticKeyBindings() keyboardpolicy.Bindings {
	return keyboardpolicy.Bindings{
		Submit:          m.keys.Submit,
		Newline:         m.keys.Newline,
		CyclePermission: m.keys.CyclePermission,
		ToggleTodo:      m.keys.ToggleTodo,
		Transcript:      m.keys.Transcript,
		ToggleSkills:    m.keys.ToggleSkills,
		ToggleModel:     m.keys.ToggleModel,
		PageUp:          m.keys.PageUp,
		PageDown:        m.keys.PageDown,
	}
}

func (m *bubbleModel) handleInterruptKey() tea.Cmd {
	if m.panes.showTranscript {
		m.closeTranscriptOverlay()
		m.requestRelayout()
		return nil
	}
	if top := m.panes.bottom.top(); top != nil && top.ID() != permissionViewID && top.ID() != slashViewID {
		if provider, ok := top.(*providerPaneView); ok {
			provider.cancelFetch()
		}
		m.panes.bottom.remove(top.ID())
		m.requestRelayout()
		return nil
	}
	if m.busy && m.turnCancel != nil {
		m.cancelActiveTurn()
		if m.conversation != nil {
			m.conversation.ClearQueue()
		}
		return nil
	}
	prompt := m.panes.bottom.prompt()
	if prompt.Value() != "" || m.panes.bottom.bashMode() {
		m.resetPrompt()
		m.setBashMode(false)
		m.syncSlashView()
		m.requestRelayout()
		return nil
	}
	return tea.Quit
}

func (m *bubbleModel) updateKey(message tea.KeyPressMsg) tea.Cmd {
	bindings := m.semanticKeyBindings()
	priorityAction := bindings.Priority(message)

	// Shift+Tab is reserved for global permission cycling, even in blocking panes.
	if priorityAction == keyboardpolicy.CyclePermission {
		m.cyclePermission()
		return nil
	}
	top := m.panes.bottom.top()
	if top != nil && top.PresentationMode() == paneBlocking {
		if handled, command := m.handlePaneKey(message); handled {
			return m.withSpinner(command)
		}
		return nil
	}
	if priorityAction != keyboardpolicy.None {
		if handled, command := m.handleGlobalAction(priorityAction, message); handled {
			return m.withSpinner(command)
		}
	}
	if handled, command := m.handlePaneKey(message); handled {
		return m.withSpinner(command)
	}
	if navigationAction := bindings.Navigation(message); navigationAction != keyboardpolicy.None {
		if handled, command := m.handleGlobalAction(navigationAction, message); handled {
			return m.withSpinner(command)
		}
	}
	return m.handlePromptKey(message)
}

func (m *bubbleModel) handlePaneKey(message tea.KeyPressMsg) (bool, tea.Cmd) {
	top := m.panes.bottom.top()
	if top == nil {
		return false, nil
	}
	if isolated, ok := top.(isolatedPaneKeyHandler); ok {
		result := isolated.HandlePaneKey(newPaneRenderContext(m), message)
		command := result.cmd
		if result.action.kind != paneActionNone {
			command = tea.Batch(command, m.applyPaneAction(result.action))
		}
		if result.handled {
			m.requestRelayout()
		}
		return result.handled, command
	}
	return false, nil
}

func (m *bubbleModel) handleGlobalAction(action keyboardpolicy.Action, message tea.KeyPressMsg) (bool, tea.Cmd) {
	switch action {
	case keyboardpolicy.Transcript:
		m.openTranscriptOverlay()
		return true, nil
	case keyboardpolicy.ToggleSkills:
		return true, m.toggleSkillsPane()
	case keyboardpolicy.ToggleModel:
		return true, m.toggleModelSetupPane()
	case keyboardpolicy.ToggleTodo:
		return true, m.toggleTodoPane()
	case keyboardpolicy.PageUp, keyboardpolicy.PageDown:
		return true, m.updateConversationViewport(message)
	default:
		return false, nil
	}
}

type composerKeyAction = keyboardpolicy.Action

const (
	composerKeyActionNone    = keyboardpolicy.None
	composerKeyActionSubmit  = keyboardpolicy.Submit
	composerKeyActionNewline = keyboardpolicy.Newline
)

func (m *bubbleModel) composerAction(message tea.KeyPressMsg) composerKeyAction {
	return m.semanticKeyBindings().Composer(message)
}

func (m *bubbleModel) handlePromptKey(message tea.KeyPressMsg) tea.Cmd {
	prompt := m.panes.bottom.prompt()
	if message.Text == "?" && prompt.Value() == "" && !m.panes.bottom.bashMode() {
		m.openShortcutsPane()
		return nil
	}
	if key.Matches(message, paneutil.Keys.Tab) && m.busy {
		return m.withSpinner(m.submit())
	}
	if key.Matches(message, paneutil.Keys.Escape) {
		if m.panes.bottom.bashMode() {
			m.setBashMode(false)
		}
		m.resetPrompt()
		m.syncSlashView()
		m.requestRelayout()
		return nil
	}
	switch m.composerAction(message) {
	case composerKeyActionNewline:
		updated, command := prompt.Update(message)
		*prompt = updated
		m.normalizeBlankComposer()
		m.syncSlashView()
		m.requestRelayout()
		return command
	case composerKeyActionSubmit:
		return m.withSpinner(m.submit())
	}
	if !m.panes.bottom.bashMode() && prompt.Value() == "" && message.Text == "!" {
		m.setBashMode(true)
		return nil
	}
	if m.panes.bottom.bashMode() && prompt.Value() == "" && key.Matches(message, composerKeys.ExitBash) {
		m.setBashMode(false)
		return nil
	}
	if key.Matches(message, composerKeys.HistoryUp) {
		lineInfo := prompt.LineInfo()
		if prompt.LineCount() == 1 || (prompt.Line() == 0 && lineInfo.RowOffset == 0 && lineInfo.ColumnOffset == 0) {
			m.historyPrevious()
			m.syncSlashView()
			return nil
		}
	}
	if key.Matches(message, composerKeys.HistoryDown) && m.panes.bottom.historyNavigating() {
		m.historyNext()
		m.syncSlashView()
		return nil
	}
	updated, command := prompt.Update(message)
	*prompt = updated
	m.normalizeBlankComposer()
	m.syncSlashView()
	m.requestRelayout()
	return command
}
