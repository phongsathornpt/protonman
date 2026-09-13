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
	var raw struct {
		SessionID string `json:"sessionId"`
		Update struct {
			Kind       string          `json:"sessionUpdate"`
			ToolCallID string          `json:"toolCallId"`
			Title      string          `json:"title"`
			Status     string          `json:"status"`
			Content    json.RawMessage `json:"content"`
			AgentID    string          `json:"agentId"`
			Profile    string          `json:"profile"`
			Task       string          `json:"task"`
			Summary    string          `json:"summary"`
		} `json:"update"`
	}
	if json.Unmarshal(event.Params, &raw) != nil || raw.SessionID == "" {
		return
	}

	if raw.Update.Kind == subagentSessionUpdate {
		a.reduceSubagentUpdate(raw.SessionID, raw.Update.AgentID, raw.Update.Profile, raw.Update.Task, raw.Update.Summary, raw.Update.Status)
		return
	}

	text := sessionUpdateText(raw.Update.Content)
	update := desktopstate.SessionUpdate{
		SessionID:  raw.SessionID,
		Kind:       raw.Update.Kind,
		ToolCallID: raw.Update.ToolCallID,
		Title:      raw.Update.Title,
		Status:     raw.Update.Status,
		Text:       text,
	}

	if raw.Update.Kind == "agent_message_chunk" {
		a.appendTranscript(raw.SessionID, text)
		return
	}
	if reduced, ok := desktopstate.TimelineEvent(update); ok {
		a.mu.Lock()
		a.state = desktopstate.Reduce(a.state, reduced)
		active := a.state.ActiveSessionID == raw.SessionID
		a.mu.Unlock()
		if active {
			a.refreshActiveView()
		}
		if raw.Update.Kind == "tool_call_update" && terminalToolStatus(raw.Update.Status) {
			a.refreshSessionContext(raw.SessionID, true)
			a.refreshSessionMemory(raw.SessionID, true)
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
		a.refreshActiveView()
	}
}

func (a *application) refreshActiveView() {
	a.mu.Lock()
	activeID := a.state.ActiveSessionID
	a.mu.Unlock()
	if activeID != "" {
		a.refreshSessionContext(activeID, false)
		a.refreshSessionMemory(activeID, false)
	}
	a.renderActiveView()
}

func (a *application) renderActiveView() {
	a.mu.Lock()
	activeID := a.state.ActiveSessionID
	busy := a.sessionBusyLocked(activeID)
	markdown := ""
	if transcript := a.transcripts[activeID]; transcript != nil {
		markdown = transcript.String()
	}
	for _, session := range a.state.Sessions {
		if session.ID == activeID {
			markdown += renderSessionContext(session.Context)
			markdown += renderMemory(session.Context.Memory)
			markdown += renderTimeline(session.Timeline)
			markdown += renderSubagents(session.Subagents)
			break
		}
	}
	a.mu.Unlock()

	fyne.Do(func() {
		a.chat.ParseMarkdown(markdown)
		a.chat.Refresh()
		if activeID == "" || busy {
			a.send.Disable()
		} else {
			a.send.Enable()
		}
		if busy {
			a.stop.Enable()
		} else {
			a.stop.Disable()
		}
	})
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
