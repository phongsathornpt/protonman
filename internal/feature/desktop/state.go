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

// TodoItemState is one revisioned durable task rendered by Desktop.
type TodoItemState struct {
	ID     string
	Text   string
	Status string
}

// TodoState is the reducer-owned projection of a session TODO snapshot.
type TodoState struct {
	Revision uint64
	Items    []TodoItemState
}

// MemoryEntryState is the read-only Desktop projection of one durable memory.
type MemoryEntryState struct {
	ID         string
	Scope      string
	Kind       string
	Key        string
	Value      string
	Confidence float64
	UsageCount uint64
}

// MemoryState contains workspace-local and global durable memory independently.
type MemoryState struct {
	WorkspaceKey string
	Workspace    []MemoryEntryState
	Global       []MemoryEntryState
}

// RuntimeSettingsState is the reducer-owned session runtime selection.
type RuntimeSettingsState struct {
	Provider       string
	Model          string
	Reasoning      string
	LowConcurrency string
}

// MCPIntegrationState is one Desktop-managed ACP MCP server definition.
type MCPIntegrationState struct {
	Name    string
	Command string
	Args    []string
	Env     []string
}

// SessionContextState contains inspectable durable goal, TODO, and memory state.
type SessionContextState struct {
	Goal   string
	Todo   TodoState
	Memory MemoryState
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
	ID                    string
	AgentID               string
	ProjectID             string
	Title                 string
	Workspace             string
	AdditionalDirectories []string
	WorkspaceKey          string
	WorkspaceName         string
	Status                TaskStatus
	Timeline              []TimelineItem
	Subagents             []SubagentState
	Context               SessionContextState
	Runtime               RuntimeSettingsState
}

// State owns desktop project and session state independently from Fyne widgets.
type State struct {
	ActiveSessionID string
	ActiveProjectID string
	Projects        []ProjectState
	Sessions        []SessionState
	PermissionInbox []PermissionRequest
	Integrations    []MCPIntegrationState
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
	EventSessionContextUpdated
	EventSessionMemoryUpdated
	EventSessionRuntimeUpdated
	EventIntegrationsReplaced
)

// Event is a typed reducer input. Only fields relevant to Kind are consumed.
type Event struct {
	Kind         EventKind
	SessionID    string
	Sessions     []SessionState
	Item         TimelineItem
	Subagent     SubagentState
	Context      SessionContextState
	Memory       MemoryState
	Runtime      RuntimeSettingsState
	Integrations []MCPIntegrationState
	Permission   PermissionRequest
	RequestID    string
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
	case EventSessionContextUpdated:
		if session := sessionByID(&next, event.SessionID); session != nil {
			memory := cloneMemoryState(session.Context.Memory)
			session.Context = cloneSessionContext(event.Context)
			session.Context.Memory = memory
		}
	case EventSessionMemoryUpdated:
		if session := sessionByID(&next, event.SessionID); session != nil {
			session.Context.Memory = cloneMemoryState(event.Memory)
		}
	case EventSessionRuntimeUpdated:
		if session := sessionByID(&next, event.SessionID); session != nil {
			session.Runtime = event.Runtime
		}
	case EventIntegrationsReplaced:
		next.Integrations = cloneIntegrations(event.Integrations)
	}

	return next
}

func cloneState(state State) State {
	state.Projects = cloneProjects(state.Projects)
	state.Sessions = cloneSessions(state.Sessions)
	state.PermissionInbox = clonePermissions(state.PermissionInbox)
	state.Integrations = cloneIntegrations(state.Integrations)
	return state
}

type ProjectFolder struct {
	Path    string
	Primary bool
}

type ProjectState struct {
	ID             string
	Name           string
	Folders        []ProjectFolder
	AgentIDs       []string
	DefaultAgentID string
}

func cloneProjects(projects []ProjectState) []ProjectState {
	out := slices.Clone(projects)
	for i := range out {
		out[i].Folders = slices.Clone(out[i].Folders)
		out[i].AgentIDs = slices.Clone(out[i].AgentIDs)
	}
	return out
}

func cloneSessions(sessions []SessionState) []SessionState {
	out := slices.Clone(sessions)
	for i := range out {
		out[i].Timeline = slices.Clone(out[i].Timeline)
		out[i].Subagents = slices.Clone(out[i].Subagents)
		out[i].AdditionalDirectories = slices.Clone(out[i].AdditionalDirectories)
		out[i].Context = cloneSessionContext(out[i].Context)
	}
	return out
}

func cloneSessionContext(context SessionContextState) SessionContextState {
	context.Todo.Items = slices.Clone(context.Todo.Items)
	context.Memory = cloneMemoryState(context.Memory)
	return context
}

func cloneMemoryState(memory MemoryState) MemoryState {
	memory.Workspace = slices.Clone(memory.Workspace)
	memory.Global = slices.Clone(memory.Global)
	return memory
}

func cloneIntegrations(items []MCPIntegrationState) []MCPIntegrationState {
	out := slices.Clone(items)
	for i := range out {
		out[i].Args = slices.Clone(out[i].Args)
		out[i].Env = slices.Clone(out[i].Env)
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
