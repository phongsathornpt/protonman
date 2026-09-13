package desktop

import "slices"

// TaskStatus is the durable UI-facing lifecycle of a desktop session turn.
type TaskStatus string

const (
	TaskIdle              TaskStatus = "idle"
	TaskQueued            TaskStatus = "queued"
	TaskRunning           TaskStatus = "running"
	TaskWaitingPermission TaskStatus = "waiting_permission"
	TaskWaitingUser       TaskStatus = "waiting_user"
	TaskPaused            TaskStatus = "paused"
	TaskCompleted         TaskStatus = "completed"
	TaskFailed            TaskStatus = "failed"
)

// TimelineKind identifies one visible conversation/progress item.
type TimelineKind string

const (
	TimelineUser       TimelineKind = "user"
	TimelineAssistant  TimelineKind = "assistant"
	TimelineTool       TimelineKind = "tool"
	TimelineSubagent   TimelineKind = "subagent"
	TimelinePermission TimelineKind = "permission"
	TimelineStatus     TimelineKind = "status"
)

// TimelineItem is presentation-neutral desktop timeline state.
type TimelineItem struct {
	Kind   TimelineKind
	ID     string
	Title  string
	Text   string
	Status string
}

// SubagentState is the desktop projection of a delegated agent participant.
type SubagentState struct {
	ID      string
	Profile string
	Task    string
	Summary string
	Status  string
}

// PermissionOption is one user-selectable decision for a pending permission request.
type PermissionOption struct {
	ID   string
	Name string
	Kind string
}

// PermissionRequest is the desktop projection of an ACP server-to-client permission request.
type PermissionRequest struct {
	RequestID string
	SessionID string
	Title     string
	Detail    string
	Options   []PermissionOption
}

// SessionState is the desktop projection of one ACP session.
type SessionState struct {
	ID        string
	Title     string
	Workspace string
	Status    TaskStatus
	Timeline  []TimelineItem
	Subagents []SubagentState
}

// State owns desktop session state independently from Fyne widgets.
type State struct {
	ActiveSessionID string
	Sessions        []SessionState
	PermissionInbox []PermissionRequest
}

// EventKind identifies a reducer transition.
type EventKind uint8

const (
	EventSessionsReplaced EventKind = iota
	EventSessionSelected
	EventPromptQueued
	EventPromptStarted
	EventPromptCompleted
	EventPromptFailed
	EventPermissionRequested
	EventPermissionResolved
	EventTimelineAppended
	EventTimelineUpserted
	EventSubagentUpserted
)

// Event is a typed reducer input. Only fields relevant to Kind are consumed.
type Event struct {
	Kind       EventKind
	SessionID  string
	Sessions   []SessionState
	Item       TimelineItem
	Subagent   SubagentState
	Permission PermissionRequest
	RequestID  string
}

// Reduce applies one event and returns a new state without aliasing caller-owned slices.
func Reduce(current State, event Event) State {
	next := cloneState(current)

	switch event.Kind {
	case EventSessionsReplaced:
		next.Sessions = cloneSessions(event.Sessions)
		if next.ActiveSessionID != "" && !hasSession(next.Sessions, next.ActiveSessionID) {
			next.ActiveSessionID = ""
		}
	case EventSessionSelected:
		if hasSession(next.Sessions, event.SessionID) {
			next.ActiveSessionID = event.SessionID
		}
	case EventPromptQueued:
		setStatus(&next, event.SessionID, TaskQueued)
	case EventPromptStarted:
		setStatus(&next, event.SessionID, TaskRunning)
	case EventPromptCompleted:
		setStatus(&next, event.SessionID, TaskCompleted)
	case EventPromptFailed:
		setStatus(&next, event.SessionID, TaskFailed)
	case EventPermissionRequested:
		setStatus(&next, event.SessionID, TaskWaitingPermission)
		if event.Permission.RequestID != "" && !hasPermission(next.PermissionInbox, event.Permission.RequestID) {
			next.PermissionInbox = append(next.PermissionInbox, clonePermission(event.Permission))
		}
	case EventPermissionResolved:
		setStatus(&next, event.SessionID, TaskRunning)
		next.PermissionInbox = removePermission(next.PermissionInbox, event.RequestID)
	case EventTimelineAppended:
		if session := sessionByID(&next, event.SessionID); session != nil {
			session.Timeline = append(session.Timeline, event.Item)
		}
	case EventTimelineUpserted:
		if session := sessionByID(&next, event.SessionID); session != nil {
			upsertTimeline(session, event.Item)
		}
	case EventSubagentUpserted:
		if session := sessionByID(&next, event.SessionID); session != nil {
			upsertSubagent(session, event.Subagent)
		}
	}

	return next
}

func cloneState(state State) State {
	state.Sessions = cloneSessions(state.Sessions)
	state.PermissionInbox = clonePermissions(state.PermissionInbox)
	return state
}

func cloneSessions(sessions []SessionState) []SessionState {
	out := slices.Clone(sessions)
	for i := range out {
		out[i].Timeline = slices.Clone(out[i].Timeline)
		out[i].Subagents = slices.Clone(out[i].Subagents)
	}
	return out
}

func clonePermissions(items []PermissionRequest) []PermissionRequest {
	out := slices.Clone(items)
	for i := range out {
		out[i] = clonePermission(out[i])
	}
	return out
}

func clonePermission(item PermissionRequest) PermissionRequest {
	item.Options = slices.Clone(item.Options)
	return item
}

func hasSession(sessions []SessionState, id string) bool {
	for i := range sessions {
		if sessions[i].ID == id {
			return true
		}
	}
	return false
}

func hasPermission(items []PermissionRequest, id string) bool {
	for i := range items {
		if items[i].RequestID == id {
			return true
		}
	}
	return false
}

func removePermission(items []PermissionRequest, id string) []PermissionRequest {
	for i := range items {
		if items[i].RequestID == id {
			return append(items[:i:i], items[i+1:]...)
		}
	}
	return items
}

func sessionByID(state *State, id string) *SessionState {
	for i := range state.Sessions {
		if state.Sessions[i].ID == id {
			return &state.Sessions[i]
		}
	}
	return nil
}

func setStatus(state *State, id string, status TaskStatus) {
	if session := sessionByID(state, id); session != nil {
		session.Status = status
	}
}

func upsertTimeline(session *SessionState, item TimelineItem) {
	if item.ID != "" {
		for i := range session.Timeline {
			if session.Timeline[i].ID == item.ID && session.Timeline[i].Kind == item.Kind {
				session.Timeline[i] = MergeTimelineItem(session.Timeline[i], item)
				return
			}
		}
	}
	session.Timeline = append(session.Timeline, item)
}

func upsertSubagent(session *SessionState, next SubagentState) {
	if next.ID != "" {
		for i := range session.Subagents {
			if session.Subagents[i].ID == next.ID {
				previous := session.Subagents[i]
				if next.Profile == "" {
					next.Profile = previous.Profile
				}
				if next.Task == "" {
					next.Task = previous.Task
				}
				if next.Summary == "" {
					next.Summary = previous.Summary
				}
				if next.Status == "" {
					next.Status = previous.Status
				}
				session.Subagents[i] = next
				return
			}
		}
	}
	session.Subagents = append(session.Subagents, next)
}
