package runtime

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"encoding/json"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"strings"
)

func (m *bubbleModel) applyTurnEvents(events []app.Event) {
	if len(events) == 0 {
		return
	}
	var pending strings.Builder
	pendingRound := 0
	flushText := func() {
		if pending.Len() == 0 {
			return
		}
		m.applyTurnEvent(app.Event{Kind: app.EventTextDelta, Round: pendingRound, Text: pending.String()})
		pending.Reset()
		pendingRound = 0
	}
	for _, event := range events {
		if event.Kind == app.EventTextDelta {
			pending.WriteString(event.Text)
			if event.Round > pendingRound {
				pendingRound = event.Round
			}
			continue
		}
		flushText()
		m.applyTurnEvent(event)
	}
	flushText()
}

func (m *bubbleModel) applyTurnEvent(event app.Event) {
	if event.Round > 0 {
		m.turnProgress.Round = event.Round
	}
	switch event.Kind {
	case app.EventTextDelta:
		m.activity = "synthesizing"
		m.appendAssistantDelta(event.Text)
	case app.EventToolCall:
		m.turnProgress.ToolCalls++
		m.appendToolCall(event.Call)
	case app.EventToolResult:
		result := event.Result
		if result.CallID == "" {
			result.CallID = event.Call.ID
		}
		if result.ToolName == "" {
			result.ToolName = event.Call.Name
		}
		m.applyToolResult(event.Call.Name, result, event.Err)
		m.syncTodoSnapshot()
		m.activity = "analyzing"
	case app.EventCompleted:
		m.ensureHistoryState().CommitActive()
	case app.EventFailed:
		m.appendTurnFailure(event.Err)
	}
}

func (m *bubbleModel) appendTurnFailure(err error) {
	if err == nil {
		return
	}
	classified := ClassifyOpenCodeError(err, m.activeProvider, m.activeModel)
	if classified.Kind == ErrorKindCancelled {
		text := "turn cancelled"
		cells := m.ensureHistoryState().Cells()
		if n := len(cells); n > 0 {
			if last, ok := cells[n-1].(*SystemCell); ok && last.Text == text {
				return
			}
		}
		m.ensureHistoryState().Append(&SystemCell{Text: text})
		return
	}
	cells := m.ensureHistoryState().Cells()
	if n := len(cells); n > 0 {
		if last, ok := cells[n-1].(*ErrorCell); ok && last.Text == classified.Message && last.Title == classified.Title {
			return
		}
	}
	m.ensureHistoryState().Append(&ErrorCell{ErrorKind: classified.Kind, Title: classified.Title, Badge: classified.Badge, Text: classified.Message, Suggestions: classified.Suggestions, RawDetails: classified.RawDetails, Retryable: classified.Retryable})
}

func (m *bubbleModel) appendTurnResult(events []app.Event, result app.Result, err error) {
	sawAssistant := false
	for _, event := range events {
		if event.Kind == app.EventTextDelta && event.Text != "" {
			sawAssistant = true
		}
		m.applyTurnEvent(event)
	}
	if !sawAssistant && result.Message.Content != "" {
		m.appendAssistant(result.Message.Content)
	}
	m.ensureHistoryState().CommitActive()
	m.appendTurnFailure(err)
}

func (m *bubbleModel) appendToolResult(result tool.Result, err error) {
	m.applyToolResult(result.ToolName, result, err)
	if err == nil {
		m.appendMuted("tool completed")
	}
}

func (m bubbleModel) renderBlocks() []string {
	if m.historyState == nil {
		return nil
	}
	return m.historyState.RenderLines()
}

func plainTranscript(model *bubbleModel) string {
	if model == nil || model.historyState == nil {
		return ""
	}
	return model.historyState.Raw()
}

func (m *bubbleModel) loadInitialMessages(messages []model.Message) {
	state := m.ensureHistoryState()
	for _, message := range messages {
		text := strings.TrimSpace(message.Content)
		switch message.Role {
		case model.RoleUser:
			if text != "" {
				if strings.HasPrefix(text, "Activated skill ") {
					if idx := strings.Index(text, "\n"); idx != -1 {
						text = text[:idx]
					}
				}
				state.Append(&UserCell{Text: text})
			}
		case model.RoleAssistant:
			if text != "" {
				state.Append(&AssistantCell{Text: message.Content})
			}
		case model.RoleTool:
			if text != "" || message.ToolName != "" {
				kind := tool.KindForName(message.ToolName)
				summary := summarizeToolOutput(message.ToolName, kind, "", message.Content, nil, false)
				state.Append(&ToolCell{Name: message.ToolName, Body: message.Content, ToolKind: kind, Summary: summary})
			}
		case model.RoleSystem:
			if text != "" {
				state.Append(&SystemCell{Text: message.Content})
			}
		}
	}
}

func extractStringArg(raw json.RawMessage, key string) string {
	call := tool.Call{Arguments: raw}
	return tool.ExtractString(call.ArgumentsMap(), key)
}

func (m *bubbleModel) closeTranscriptOverlay() {
	if m == nil {
		return
	}
	m.showTranscript = false
	if m.historyState != nil {
		m.historyState.ReleaseAlternateRenderCache()
		m.historyState.ReleaseRawTextCache()
	}
	m.transcriptViewport.SetContent("")
}

func (m *bubbleModel) refreshTranscriptViewport(forceTail bool) {
	if m.historyState == nil {
		return
	}
	follow := forceTail || m.transcriptViewport.AtBottom()
	content := m.historyState.Raw()
	if !m.rawTranscript {
		content = strings.Join(m.historyState.RenderLinesAt(maxInt(8, m.transcriptViewport.Width())), "\n")
	}
	if strings.TrimSpace(content) == "" {
		content = mutedStyle.Render("No transcript yet.")
	}
	m.transcriptViewport.SetContent(content)
	if follow {
		m.transcriptViewport.GotoBottom()
	}
}

func (m *bubbleModel) transcriptOverlayView() string {
	mode := "rich"
	if m.rawTranscript {
		mode = "raw"
	}
	header := brandStyle.Render("Transcript") + mutedStyle.Render(" · "+mode)
	footer := mutedStyle.Render("esc/ctrl+t close · r raw/rich · pgup/pgdn scroll")
	body := lipgloss.JoinVertical(lipgloss.Left, header, m.transcriptViewport.View(), footer)
	width := maxInt(1, m.width-6)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentAssistant).Padding(0, 1).Width(width).Render(body)
}

func (m *bubbleModel) updateTranscriptKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(message, m.keys.Transcript) {
		m.closeTranscriptOverlay()
		return m, nil
	}
	switch message.String() {
	case "esc", "q":
		m.closeTranscriptOverlay()
		return m, nil
	case "r":
		m.rawTranscript = !m.rawTranscript
		if m.historyState != nil {
			if m.rawTranscript {
				m.historyState.ReleaseAlternateRenderCache()
			} else {
				m.historyState.ReleaseRawTextCache()
			}
		}
		m.refreshTranscriptViewport(false)
		return m, nil
	case "pgup":
		m.transcriptViewport.PageUp()
		return m, nil
	case "pgdown":
		m.transcriptViewport.PageDown()
		return m, nil
	case "up", "k":
		m.transcriptViewport.ScrollUp(1)
		return m, nil
	case "down", "j":
		m.transcriptViewport.ScrollDown(1)
		return m, nil
	case "home", "g":
		m.transcriptViewport.GotoTop()
		return m, nil
	case "end", "G":
		m.transcriptViewport.GotoBottom()
		return m, nil
	default:
		return m, nil
	}
}
