package runtime

import (
	"os"
	"strings"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/clipboardimage"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/transientnotice"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
)

type clipboardImageLoadedMsg struct {
	draftText        string
	draftAttachments int
	path             string
	width            int
	height           int
	err              error
}

func writeClipboardTempPNG(raw []byte) (string, error) {
	return clipboardimage.WriteTempPNG(raw)
}

func loadClipboardImage(draftText string, draftAttachments int) tea.Cmd {
	return func() tea.Msg {
		loaded := clipboardimage.Load()
		return clipboardImageLoadedMsg{
			draftText:        draftText,
			draftAttachments: draftAttachments,
			path:             loaded.Path,
			width:            loaded.Width,
			height:           loaded.Height,
			err:              loaded.Err,
		}
	}
}

func (m *bubbleModel) beginClipboardImagePaste() tea.Cmd {
	if m == nil || m.panes.bottom == nil || !m.panes.bottom.composerVisible() || m.panes.bottom.prompt() == nil {
		return nil
	}
	if !m.currentModelAcceptsImageInput() {
		m.appendError(m.imageInputsNotSupportedMessage())
		m.requestRelayout()
		return nil
	}
	prompt := m.panes.bottom.prompt()
	return loadClipboardImage(prompt.Value(), len(m.panes.bottom.composer.attachments.localImages))
}

func (m *bubbleModel) updateClipboardImageLoaded(message clipboardImageLoadedMsg) tea.Cmd {
	if m == nil || m.panes.bottom == nil || !m.panes.bottom.composerVisible() || m.panes.bottom.prompt() == nil {
		if message.path != "" {
			_ = os.Remove(message.path)
		}
		return nil
	}
	prompt := m.panes.bottom.prompt()
	if prompt.Value() != message.draftText || len(m.panes.bottom.composer.attachments.localImages) != message.draftAttachments {
		if message.path != "" {
			_ = os.Remove(message.path)
		}
		return nil
	}
	if message.err != nil {
		m.appendError(message.err.Error())
		m.requestRelayout()
		return nil
	}
	if !m.currentModelAcceptsImageInput() {
		_ = os.Remove(message.path)
		m.appendError(m.imageInputsNotSupportedMessage())
		m.requestRelayout()
		return nil
	}
	m.panes.bottom.composer.attachments.attachTemporaryImage(prompt, message.path)
	m.syncSlashView()
	m.requestRelayout()
	return nil
}

func (m *bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer m.reconcileLayout()
	if command, handled := m.updateTerminalEvent(msg); handled {
		return m, command
	}
	if command, handled := m.updateAnimationEvent(msg); handled {
		return m, command
	}
	if command, handled := m.updateRuntimeEvent(msg); handled {
		return m, command
	}
	if command, handled := m.updatePaneEvent(msg); handled {
		return m, command
	}
	return m, nil
}

func (m *bubbleModel) updateTerminalEvent(msg tea.Msg) (tea.Cmd, bool) {
	switch message := msg.(type) {
	case tea.KeyboardEnhancementsMsg:
		m.updateKeyboardCapability(message)
		m.requestRelayout()
		return nil, true
	case tea.WindowSizeMsg:
		m.resize(message.Width, message.Height)
		return nil, true
	case tea.PasteMsg:
		if m.panes.showTranscript || m.panes.bottom == nil {
			return nil, true
		}
		if top := m.panes.bottom.top(); top != nil {
			if handler, ok := top.(isolatedPanePasteHandler); ok {
				result := handler.HandlePanePaste(newPaneRenderContext(m), message)
				command := result.cmd
				if result.action.kind != paneActionNone {
					command = tea.Batch(command, m.applyPaneAction(result.action))
				}
				if result.handled {
					m.requestRelayout()
				}
				return command, true
			}
		}
		if !m.panes.bottom.composerVisible() {
			return nil, true
		}
		prompt := m.panes.bottom.prompt()
		if prompt == nil {
			return nil, true
		}
		if path, ok := localImagePathFromPaste(message.Content, m.workDir); ok {
			m.panes.bottom.attachImage(path)
			m.syncSlashView()
			m.requestRelayout()
			return nil, true
		}
		cleaned := normalizePastedPath(message.Content, m.workDir)
		updated, command := prompt.Update(tea.PasteMsg{Content: cleaned})
		*prompt = updated
		m.syncSlashView()
		m.requestRelayout()
		return command, true
	case tea.KeyPressMsg:
		m.debugKeyPress(message)
		if key.Matches(message, m.keys.Quit) {
			return m.handleInterruptKey(), true
		}
		if m.panes.showTranscript {
			return m.updateTranscriptKey(message), true
		}
		return m.updateKey(message), true
	case tea.MouseMsg:
		return m.updateMouseEvent(message), true
	default:
		return nil, false
	}
}

func (m *bubbleModel) updateMouseEvent(message tea.MouseMsg) tea.Cmd {
	mouse := message.Mouse()
	if m.panes.showTranscript {
		var command tea.Cmd
		m.panes.transcript, command = m.panes.transcript.Update(message)
		return command
	}
	if top := m.panes.bottom.top(); top != nil {
		if view, ok := top.(isolatedPaneKeyHandler); ok {
			var keyCode rune
			switch mouse.Button {
			case tea.MouseWheelUp:
				keyCode = tea.KeyUp
			case tea.MouseWheelDown:
				keyCode = tea.KeyDown
			}
			if keyCode != 0 {
				result := view.HandlePaneKey(newPaneRenderContext(m), tea.KeyPressMsg{Code: keyCode})
				m.requestRelayout()
				return result.cmd
			}
		}
	}
	_, clicked := message.(tea.MouseClickMsg)
	if clicked && mouse.Button == tea.MouseLeft && m.panes.bottom.composerVisible() {
		if top, bottom, ok := m.composerMouseRegion(); ok && mouse.Y >= top && mouse.Y < bottom {
			if prompt := m.panes.bottom.prompt(); prompt != nil {
				_ = prompt.Focus()
				return nil
			}
		}
	}
	if mouse.Y < 0 || mouse.Y >= m.viewport.Height() {
		return nil
	}
	return m.updateConversationViewport(message)
}

func (m *bubbleModel) composerMouseRegion() (int, int, bool) {
	if m == nil || m.layout.frame.composer == "" {
		return 0, 0, false
	}
	composerHeight := lipgloss.Height(m.layout.frame.composer)
	parts := make([]string, 0, 6)
	parts = appendNonEmptyFramePart(parts, m.layout.frame.header)
	parts = appendNonEmptyFramePart(parts, strings.Repeat(" ", m.viewport.Height()))
	parts = appendNonEmptyFramePart(parts, m.layout.frame.divider)
	parts = appendNonEmptyFramePart(parts, m.layout.frame.status)
	if top := m.panes.bottom.top(); top == nil || top.PresentationMode() != paneBelowComposer {
		parts = appendNonEmptyFramePart(parts, m.layout.frame.top)
	}
	top := framePartsHeight(parts)
	return top, top + composerHeight, composerHeight > 0
}

func appendNonEmptyFramePart(parts []string, part string) []string {
	if part == "" {
		return parts
	}
	return append(parts, part)
}

func framePartsHeight(parts []string) int {
	height := 0
	for _, part := range parts {
		if height > 0 {
			height++
		}
		height += lipgloss.Height(part)
	}
	return height
}

func (m *bubbleModel) updateAnimationEvent(msg tea.Msg) (tea.Cmd, bool) {
	switch message := msg.(type) {
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(message)
		if !m.busy {
			return nil, true
		}
		m.refreshStatusFrame()
		return command, true
	case cursor.BlinkMsg:
		if m.reducedMotion {
			return nil, true
		}
		if top := m.panes.bottom.top(); top != nil {
			if handler, ok := top.(isolatedPaneMsgHandler); ok {
				result := handler.HandlePaneMsg(newPaneRenderContext(m), message)
				if result.handled {
					m.refreshFrameLayout()
					return result.cmd, true
				}
			}
		}
		prompt := m.panes.bottom.prompt()
		if prompt != nil {
			updated, command := prompt.Update(message)
			*prompt = updated
			m.refreshComposerFrame()
			return command, true
		}
		return nil, true
	default:
		return nil, false
	}
}

func (m *bubbleModel) updatePaneEvent(msg tea.Msg) (tea.Cmd, bool) {
	if m.panes.bottom == nil {
		return nil, false
	}
	if top := m.panes.bottom.top(); top != nil {
		if handler, ok := top.(isolatedPaneMsgHandler); ok {
			result := handler.HandlePaneMsg(newPaneRenderContext(m), msg)
			if result.handled {
				command := result.cmd
				if result.action.kind != paneActionNone {
					command = tea.Batch(command, m.applyPaneAction(result.action))
				}
				m.requestRelayout()
				return command, true
			}
		}
	} else if m.panes.bottom.composerVisible() {
		if prompt := m.panes.bottom.prompt(); prompt != nil {
			oldVal := prompt.Value()
			updated, cmd := prompt.Update(msg)
			*prompt = updated
			if prompt.Value() != oldVal || cmd != nil {
				m.syncSlashView()
				m.requestRelayout()
				return cmd, true
			}
		}
	}
	return nil, false
}

func (m *bubbleModel) updateRuntimeEvent(msg tea.Msg) (tea.Cmd, bool) {
	switch message := msg.(type) {
	case agentLifecycleMsg:
		return m.updateAgentLifecycle(message), true
	case permissionRequestMsg:
		return m.updatePermissionRequest(message), true
	case permissionBridgeClosedMsg:
		return nil, true
	case toolResultMsg:
		return m.updateToolResult(message), true
	case todoReloadedMsg:
		return m.updateTodoReloaded(message), true
	case modelsFetchedMsg:
		return m.updateModelsFetched(message), true
	case providerSavedMsg:
		return m.updateProviderSaved(message), true
	case modelSetupAppliedMsg:
		return m.updateModelSetupApplied(message), true
	case providerActiveSelectedMsg:
		return m.updateProviderActiveSelected(message), true
	case providerDeletedMsg:
		return m.updateProviderDeleted(message), true
	case permissionRuleSavedMsg:
		return m.updatePermissionRuleSaved(message), true
	case clipboardImageLoadedMsg:
		return m.updateClipboardImageLoaded(message), true
	case imageSubmissionPreparedMsg:
		return m.updateImageSubmissionPrepared(message), true
	case transientnotice.Expired:
		if message.ID == m.transientNoticeID {
			m.transientNotice = ""
			m.refreshStatusFrame()
		}
		return nil, true
	case turnmsg.Delta:
		return m.updateTurnDelta(message), true
	case turnmsg.EventsClosed:
		return m.updateTurnEventsClosed(message), true
	case turnmsg.Done:
		return m.updateTurnDone(message), true
	default:
		return nil, false
	}
}
