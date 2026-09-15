//go:build desktop

package desktop

import (
	"encoding/json"
	"strings"

	"fyne.io/fyne/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const subagentSessionUpdate = "protonman_subagent_update"

func (a *application) handleEvent(event acpclient.Event) {
	if event.Method != "session/update" {
		return
	}
	sessionID, kind, rawUpdate, ok := decodeSessionUpdate(event.Params)
	if !ok {
		return
	}

	switch kind {
	case subagentSessionUpdate:
		var update subagentUpdate
		if json.Unmarshal(rawUpdate, &update) == nil {
			a.reduceSubagentUpdate(sessionID, update.AgentID, update.Profile, update.Task, update.Summary, update.Status)
		}
		return
	case "user_message_chunk", "agent_message_chunk":
		var update messageChunkUpdate
		if json.Unmarshal(rawUpdate, &update) != nil {
			return
		}
		text := sessionUpdateText(update.Content)
		if kind == "user_message_chunk" {
			a.appendTranscript(sessionID, formatUserTranscript(text))
		} else {
			a.appendTranscript(sessionID, text)
		}
		return
	case "config_option_update":
		var update configOptionUpdate
		if json.Unmarshal(rawUpdate, &update) == nil {
			a.applySessionConfigOptions(sessionID, update.ConfigOptions)
		}
		return
	case "session_info_update":
		var update sessionInfoUpdate
		if json.Unmarshal(rawUpdate, &update) == nil {
			a.applySessionInfoUpdate(sessionID, update)
		}
		return
	case "current_mode_update":
		var update currentModeUpdate
		if json.Unmarshal(rawUpdate, &update) == nil {
			a.applyCurrentModeUpdate(sessionID, update)
		}
		return
	case "available_commands_update":
		var update availableCommandUpdate
		if json.Unmarshal(rawUpdate, &update) == nil {
			a.applyAvailableCommandsUpdate(sessionID, update)
		}
		return
	case "usage_update":
		var update usageUpdate
		if json.Unmarshal(rawUpdate, &update) == nil {
			a.applyUsageUpdate(sessionID, update)
		}
		return
	case "tool_call", "tool_call_update":
		var wire toolUpdate
		if json.Unmarshal(rawUpdate, &wire) != nil {
			return
		}
		update := desktopstate.SessionUpdate{
			SessionID: sessionID, Kind: kind, ToolCallID: wire.ToolCallID,
			Title: wire.Title, Status: wire.Status, Text: sessionUpdateText(wire.Content),
		}
		if reduced, ok := desktopstate.TimelineEvent(update); ok {
			a.mu.Lock()
			a.state = desktopstate.Reduce(a.state, reduced)
			active := a.state.ActiveSessionID == sessionID
			a.mu.Unlock()
			if active {
				a.refreshActiveView()
			}
			if kind == "tool_call_update" && terminalToolStatus(wire.Status) {
				a.refreshSessionContext(sessionID, true)
				a.refreshSessionMemory(sessionID, true)
			}
		}
	}
}

func (a *application) reduceSubagentUpdate(sessionID, agentID, profile, task, summary, status string) {
	if strings.TrimSpace(agentID) == "" {
		return
	}
	a.mu.Lock()
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{
		Kind:      desktopstate.EventSubagentUpserted,
		SessionID: sessionID,
		Subagent: desktopstate.SubagentState{
			ID:      agentID,
			Profile: strings.ToLower(strings.TrimSpace(profile)),
			Task:    strings.TrimSpace(task),
			Summary: strings.TrimSpace(summary),
			Status:  strings.TrimSpace(status),
		},
	})
	active := a.state.ActiveSessionID == sessionID
	a.mu.Unlock()
	if active {
		a.refreshActiveView()
	}
}

func sessionUpdateText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var block struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &block) == nil && block.Text != "" {
		return block.Text
	}
	var blocks []struct {
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var out strings.Builder
	for _, item := range blocks {
		out.WriteString(item.Content.Text)
	}
	return out.String()
}

func (a *application) appendTranscript(sessionID, text string) {
	if text == "" {
		return
	}
	if stageSessionHistoryChunk(a, sessionID, text) {
		return
	}
	a.mu.Lock()
	builder := a.transcripts[sessionID]
	if builder == nil {
		builder = &strings.Builder{}
		a.transcripts[sessionID] = builder
	}
	builder.WriteString(text)
	active := a.state.ActiveSessionID == sessionID
	a.mu.Unlock()
	if active {
		a.scheduleTranscriptRender()
	}
}

func (a *application) refreshActiveView() {
	a.mu.Lock()
	activeID := a.state.ActiveSessionID
	a.mu.Unlock()
	if activeID != "" {
		a.refreshSessionContext(activeID, false)
		a.refreshSessionMemory(activeID, false)
		a.refreshSessionRuntime(activeID, false)
	}
	a.renderActiveView()
	a.renderRuntimeControls()
}

func (a *application) renderActiveView() {
	a.mu.Lock()
	activeID := a.state.ActiveSessionID
	promptBusy := a.sessionBusyLocked(activeID)
	sessionChanged := activeID != a.conversationSessionID
	a.conversationSessionID = activeID
	transcript := ""
	if current := a.transcripts[activeID]; current != nil {
		transcript = current.String()
	}
	markdown := transcript
	for _, session := range a.state.Sessions {
		if session.ID == activeID {
			markdown = renderConversation(transcript, session)
			break
		}
	}
	a.mu.Unlock()
	historyLoading := sessionHistoryIsLoading(a, activeID)

	fyne.Do(func() {
		followTail := sessionChanged || a.shouldFollowConversationTail()
		a.chat.ParseMarkdown(markdown)
		a.chat.Refresh()
		if followTail {
			a.scrollConversationToBottom()
		}
		if activeID == "" || promptBusy || historyLoading {
			a.send.Disable()
		} else {
			a.send.Enable()
		}
		if promptBusy {
			a.stop.Enable()
		} else {
			a.stop.Disable()
		}
	})
	a.renderSessionChrome()
}

// renderConversation intentionally excludes goal, TODO and durable memory.
// Those are inspector state, not conversation content, and rendering them inline
// makes the primary chat surface behave like a debug dump.
func renderConversation(transcript string, session desktopstate.SessionState) string {
	var out strings.Builder
	out.WriteString(transcript)
	out.WriteString(renderTimeline(session.Timeline))
	out.WriteString(renderSubagents(session.Subagents))
	return out.String()
}

func terminalToolStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "canceled", "interrupted":
		return true
	default:
		return false
	}
}

func renderTimeline(items []desktopstate.TimelineItem) string {
	var out strings.Builder
	for _, item := range items {
		if item.Kind != desktopstate.TimelineTool {
			continue
		}
		if out.Len() == 0 {
			out.WriteString("\n\n### Activity\n")
		}
		marker := statusMarker(item.Status)
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = "Tool call"
		}
		out.WriteString("\n`" + marker + " " + title + "`")
		if detail := strings.TrimSpace(item.Text); detail != "" {
			out.WriteString("\n\n" + detail)
		}
	}
	return out.String()
}

func renderSubagents(items []desktopstate.SubagentState) string {
	if len(items) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("\n\n### Teammates\n")
	for _, item := range items {
		label := subagentLabel(item.Profile)
		out.WriteString("\n**" + statusMarker(item.Status) + " " + label + "**")
		if status := strings.TrimSpace(item.Status); status != "" {
			out.WriteString(" · `" + status + "`")
		}
		if task := strings.TrimSpace(item.Task); task != "" {
			out.WriteString("\n\n" + task)
		}
		if summary := strings.TrimSpace(item.Summary); summary != "" && summary != item.Task {
			out.WriteString("\n\n_" + summary + "_")
		}
	}
	return out.String()
}

func subagentLabel(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "strength":
		return "STRENGTH"
	case "agility":
		return "AGILITY"
	case "intelligence":
		return "INTELLIGENCE"
	default:
		return "SUBAGENT"
	}
}

func statusMarker(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return "✓"
	case "failed", "canceled", "interrupted":
		return "×"
	case "queued":
		return "○"
	default:
		return "◐"
	}
}
