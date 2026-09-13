//go:build desktop

package desktop

import (
	"encoding/json"
	"strings"

	"fyne.io/fyne/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func (a *application) handleEvent(event acpclient.Event) {
	if event.Method != "session/update" {
		return
	}
	var payload struct {
		SessionID string `json:"sessionId"`
		Update struct {
			Kind       string `json:"sessionUpdate"`
			ToolCallID string `json:"toolCallId"`
			Title      string `json:"title"`
			Status     string `json:"status"`
			Content    []struct {
				Content struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"content"`
			MessageContent struct {
				Text string `json:"text"`
			} `json:"-"`
		} `json:"update"`
	}
	var raw struct {
		SessionID string `json:"sessionId"`
		Update struct {
			Kind       string `json:"sessionUpdate"`
			ToolCallID string `json:"toolCallId"`
			Title      string `json:"title"`
			Status     string `json:"status"`
			Content json.RawMessage `json:"content"`
		} `json:"update"`
	}
	if json.Unmarshal(event.Params, &raw) != nil || raw.SessionID == "" {
		return
	}

	text := sessionUpdateText(raw.Update.Content)
	update := desktopstate.SessionUpdate{
		SessionID: raw.SessionID,
		Kind: raw.Update.Kind,
		ToolCallID: raw.Update.ToolCallID,
		Title: raw.Update.Title,
		Status: raw.Update.Status,
		Text: text,
	}

	if raw.Update.Kind == "agent_message_chunk" {
		a.appendTranscript(raw.SessionID, text)
		return
	}
	if event, ok := desktopstate.TimelineEvent(update); ok {
		a.mu.Lock()
		a.state = desktopstate.Reduce(a.state, event)
		active := a.state.ActiveSessionID == raw.SessionID
		a.mu.Unlock()
		if active {
			a.refreshActiveView()
		}
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
	busy := a.sessionBusyLocked(activeID)
	markdown := ""
	if transcript := a.transcripts[activeID]; transcript != nil {
		markdown = transcript.String()
	}
	for _, session := range a.state.Sessions {
		if session.ID == activeID {
			markdown += renderTimeline(session.Timeline)
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

func renderTimeline(items []desktopstate.TimelineItem) string {
	var out strings.Builder
	for _, item := range items {
		if item.Kind != desktopstate.TimelineTool {
			continue
		}
		if out.Len() == 0 {
			out.WriteString("\n\n### Activity\n")
		}
		marker := "◐"
		switch item.Status {
		case "completed":
			marker = "✓"
		case "failed":
			marker = "×"
		}
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
