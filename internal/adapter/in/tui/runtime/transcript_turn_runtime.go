package runtime

import (
	"encoding/json"
	"strings"

	tuihistory "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	tuipresentation "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/presentation"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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
		m.turnProgress.Retry = sdk.RetryEvent{}
		m.activity = ""
		m.appendAssistantDelta(event.Text)
	case app.EventRetryScheduled:
		m.turnProgress.Retry = event.Retry
		m.activity = "retrying"
	case app.EventToolCall:
		m.turnProgress.Retry = sdk.RetryEvent{}
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
		m.activity = ""
	case app.EventCompleted:
		m.turnProgress.Retry = sdk.RetryEvent{}
		m.ensureHistoryState().CommitActive()
	case app.EventFailed:
		m.turnProgress.Retry = sdk.RetryEvent{}
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
			if last, ok := cells[n-1].(*tuihistory.SystemCell); ok && last.Text == text {
				return
			}
		}
		m.ensureHistoryState().Append(&tuihistory.SystemCell{Text: text})
		return
	}
	cells := m.ensureHistoryState().Cells()
	if n := len(cells); n > 0 {
		if last, ok := cells[n-1].(*tuihistory.ErrorCell); ok && last.Text == classified.Message && last.Title == classified.Title {
			return
		}
	}
	m.ensureHistoryState().Append(&tuihistory.ErrorCell{ErrorKind: classified.Kind, Title: classified.Title, Badge: classified.Badge, Text: classified.Message, Suggestions: classified.Suggestions, RawDetails: classified.RawDetails, Retryable: classified.Retryable})
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
				state.Append(&tuihistory.UserCell{Text: text})
			}
		case model.RoleAssistant:
			if text != "" {
				state.Append(&tuihistory.AssistantCell{Text: message.Content})
			}
		case model.RoleTool:
			if text != "" || message.ToolName != "" {
				kind := tool.KindForName(message.ToolName)
				summary := toolview.SummarizeOutput(message.ToolName, kind, "", message.Content, nil, false)
				state.Append(&tuihistory.ToolCell{Name: message.ToolName, Body: message.Content, ToolKind: kind, Summary: summary, ShowDetail: tuipresentation.MinimalPolicy().ToolDetail(kind, false, false) != tuipresentation.DetailSummary})
			}
		case model.RoleSystem:
			if text != "" {
				state.Append(&tuihistory.SystemCell{Text: message.Content})
			}
		}
	}
}

func extractStringArg(raw json.RawMessage, key string) string {
	call := tool.Call{Arguments: raw}
	return tool.ExtractString(call.ArgumentsMap(), key)
}
