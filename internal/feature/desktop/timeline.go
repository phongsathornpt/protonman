package desktop

import "strings"

// SessionUpdate is the presentation-neutral projection of an ACP session/update.
// Adapters decode ACP wire payloads into this type before reducing desktop state.
type SessionUpdate struct {
	SessionID  string
	Kind       string
	ToolCallID string
	Title      string
	Status     string
	Text       string
}

// TimelineEvent converts a normalized ACP session update into a reducer event.
// The bool is false for updates that do not affect the visible timeline.
func TimelineEvent(update SessionUpdate) (Event, bool) {
	if strings.TrimSpace(update.SessionID) == "" {
		return Event{}, false
	}

	switch update.Kind {
	case "user_message_chunk":
		if update.Text == "" {
			return Event{}, false
		}
		return Event{
			Kind:      EventTimelineAppended,
			SessionID: update.SessionID,
			Item: TimelineItem{
				Kind: TimelineUser,
				Text: update.Text,
			},
		}, true

	case "agent_message_chunk":
		if update.Text == "" {
			return Event{}, false
		}
		return Event{
			Kind:      EventTimelineAppended,
			SessionID: update.SessionID,
			Item: TimelineItem{
				Kind: TimelineAssistant,
				Text: update.Text,
			},
		}, true

	case "tool_call", "tool_call_update":
		if update.ToolCallID == "" {
			return Event{}, false
		}
		return Event{
			Kind:      EventTimelineUpserted,
			SessionID: update.SessionID,
			Item: TimelineItem{
				Kind:   TimelineTool,
				ID:     update.ToolCallID,
				Title:  update.Title,
				Text:   update.Text,
				Status: update.Status,
			},
		}, true
	}

	return Event{}, false
}

// MergeTimelineItem preserves useful fields when a terminal update omits data
// already supplied by the initial tool_call notification.
func MergeTimelineItem(previous, next TimelineItem) TimelineItem {
	if next.ID == "" {
		next.ID = previous.ID
	}
	if next.Kind == "" {
		next.Kind = previous.Kind
	}
	if next.Title == "" {
		next.Title = previous.Title
	}
	if next.Text == "" {
		next.Text = previous.Text
	}
	if next.Status == "" {
		next.Status = previous.Status
	}
	return next
}
