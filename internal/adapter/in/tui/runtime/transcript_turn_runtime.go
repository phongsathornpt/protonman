package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tuihistory "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	tuipresentation "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/presentation"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/toolview"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
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
	case app.EventReasoningDelta:
		m.turnProgress.Retry = domain.RetryEvent{}
		m.activity = "reasoning"
		m.appendReasoningDelta(event.Text)
	case app.EventTextDelta:
		m.finalizeActiveReasoning()
		m.turnProgress.Retry = domain.RetryEvent{}
		m.activity = ""
		m.appendAssistantDelta(event.Text)
	case app.EventRetryScheduled:
		m.turnProgress.Retry = event.Retry
		m.activity = "retrying"
	case app.EventToolCall:
		m.finalizeActiveReasoning()
		m.turnProgress.Retry = domain.RetryEvent{}
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
		m.activity = "thinking"
	case app.EventMemoryActivity:
		m.appendMemoryActivity(event.MemoryActivity)
	case app.EventCompleted:
		m.finalizeActiveReasoning()
		m.turnProgress.Retry = domain.RetryEvent{}
		m.ensureHistoryState().CommitActive()
	case app.EventFailed:
		m.finalizeActiveReasoning()
		m.turnProgress.Retry = domain.RetryEvent{}
		m.appendTurnFailure(event.Err)
	}
}

func (m *bubbleModel) appendReasoningDelta(text string) {
	if text == "" {
		return
	}
	history := m.ensureHistoryState()
	reasoning := history.ActiveReasoning()
	if reasoning == nil {
		reasoning = history.StartReasoning(m.spinnerIndicator(), m.icons)
	}
	reasoning.Content += text
	history.InvalidateCache()
	m.requestRelayout()
}

func (m *bubbleModel) finalizeActiveReasoning() {
	history := m.ensureHistoryState()
	if reasoning := history.ActiveReasoning(); reasoning != nil && reasoning.Streaming {
		reasoning.Streaming = false
		reasoning.SetExpanded(false)
		if reasoning.Duration == 0 && !reasoning.StartedAt.IsZero() {
			reasoning.Duration = time.Since(reasoning.StartedAt)
		}
		history.CommitActive()
		m.requestRelayout()
	}
}

func (m *bubbleModel) appendMemoryActivity(activity corememory.Activity) {
	// Memory retrieval is a routine part of eligible model requests. Reporting it
	// in the transcript on every user turn adds noise without changing the
	// conversation, so keep reads internal and surface only save/failure events.
	if activity.Kind == corememory.ActivityContextIncluded {
		return
	}
	text := memoryActivityText(activity)
	if text == "" {
		return
	}
	m.ensureHistoryState().Append(&tuihistory.MemoryActivityCell{Text: text})
}

func memoryActivityText(activity corememory.Activity) string {
	total := activity.WorkspaceEntries + activity.GlobalEntries
	plural := "items"
	if total == 1 {
		plural = "item"
	}
	scopes := make([]string, 0, 2)
	if activity.WorkspaceEntries > 0 {
		scopes = append(scopes, fmt.Sprintf("workspace %d", activity.WorkspaceEntries))
	}
	if activity.GlobalEntries > 0 {
		scopes = append(scopes, fmt.Sprintf("global %d", activity.GlobalEntries))
	}
	scopeSummary := strings.Join(scopes, ", ")
	switch activity.Kind {
	case corememory.ActivityContextUnavailable:
		return "Saved context could not be loaded; this request continues without it"
	case corememory.ActivityEntriesSaved:
		return fmt.Sprintf("Saved %d memory %s from prior sessions (%s)", total, plural, scopeSummary)
	case corememory.ActivityUpdateFailed:
		return "Background memory update failed; no successful save was confirmed"
	default:
		return ""
	}
}

func (m *bubbleModel) appendTurnFailure(err error) {
	if err == nil {
		return
	}
	classified := ClassifyOpenCodeError(err, m.activeProvider, m.activeModel)
	if classified.Kind == ErrorKindCancelled {
		text := "turn cancelled"
		if last, ok := m.ensureHistoryState().LastCell().(*tuihistory.SystemCell); ok && last.Text == text {
			return
		}
		m.ensureHistoryState().Append(&tuihistory.SystemCell{Text: text})
		return
	}
	if last, ok := m.ensureHistoryState().LastCell().(*tuihistory.ErrorCell); ok && last.Text == classified.Message && last.Title == classified.Title {
		return
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
				cleanText := text
				if strings.HasPrefix(cleanText, "[x] ") {
					cleanText = strings.TrimPrefix(cleanText, "[x] ")
				}
				if strings.HasPrefix(cleanText, "Activated skill ") {
					if idx := strings.Index(text, "\n"); idx != -1 {
						text = text[:idx]
					}
				}
				state.Append(&tuihistory.UserCell{Text: text, Icons: m.icons})
			}
		case model.RoleAssistant:
			if text != "" {
				state.Append(&tuihistory.AssistantCell{Text: message.Content})
			}
		case model.RoleTool:
			if text != "" || message.ToolName != "" {
				kind := tool.KindForName(message.ToolName)
				target := ""
				if strings.TrimSpace(message.ToolName) == tool.NameSkill {
					if skillName := toolview.ExtractSkillContentName(message.Content); skillName != "" {
						target = fmt.Sprintf("%q", skillName)
					}
				}
				summary := toolview.SummarizeOutput(message.ToolName, kind, target, message.Content, nil, false)
				state.Append(&tuihistory.ToolCell{CallID: message.ToolCallID, Name: message.ToolName, Body: message.Content, Target: target, ToolKind: kind, Summary: summary, ShowDetail: tuipresentation.MinimalPolicy().ToolDetail(kind, false, false) != tuipresentation.DetailSummary, Icons: m.icons})
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
