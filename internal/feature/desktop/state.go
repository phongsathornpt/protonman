package desktop

import (
	"slices"
	"time"
)

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
	Kind      TimelineKind
	ID        string
	Title     string
	Text      string
	Status    string
	Streaming bool
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
	PermissionMode string
}

// MCPIntegrationState is one Desktop-managed ACP MCP server definition.
type MCPIntegrationState struct {
	Name    string
	Command string
	Args    []string
	Env     []string
}

// ProviderState is the desktop projection of a model provider.
type ProviderState struct {
	ID           string
	Name         string
	Protocol     string
	BaseURL      string
	RequiresKey  bool
	IsConfigured bool
	IsActive     bool
	IsFree       bool
	HasKey       bool
	DefaultModel string
}

// SessionContextState contains inspectable durable goal, TODO, and memory state.
type SessionContextState struct {
	Goal   string
	Todo   TodoState
	Memory MemoryState
}

// SessionRef is the stable desktop identity for an ACP-owned session. ACP
// session IDs are scoped to one agent process and may collide across agents.
type SessionRef struct {
	AgentID   string
	SessionID string
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
	AgentID   string
	SessionID string
	Title     string
	Detail    string
	ToolName  string
	Command   string
	Risk      string
	RawJSON   string
	Options   []PermissionOption
}

// QuestionItemState defines a single question in an interactive question request.
type QuestionItemState struct {
	Question    string
	Options     []string
	Multiple    bool
	Recommended string
}

// QuestionRequest is the desktop projection of an ACP server-to-client question request.
type QuestionRequest struct {
	RequestID string
	AgentID   string
	SessionID string
	Questions []QuestionItemState
}

// QuestionAnswerItem represents the answer to a single question.
type QuestionAnswerItem struct {
	Question        string   `json:"question"`
	Answer          string   `json:"answer"`
	SelectedOptions []string `json:"selectedOptions,omitempty"`
}

// QuestionResponse represents the user's answer or decline to a question request.
type QuestionResponse struct {
	Status          string               `json:"status"` // "answered" | "declined"
	Answer          string               `json:"answer,omitempty"`
	SelectedOptions []string             `json:"selectedOptions,omitempty"`
	Answers         []QuestionAnswerItem `json:"answers,omitempty"`
}

// SkillState is the desktop projection of an Agent Skill.
type SkillState struct {
	Name        string
	Description string
	Scope       string
	Active      bool
	Locked      bool
	LockStatus  string
	Resources   []string
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
	LastActivityAt        time.Time
	Status                TaskStatus
	Timeline              []TimelineItem
	HistoryTruncated      bool
	Subagents             []SubagentState
	Context               SessionContextState
	Runtime               RuntimeSettingsState
	Skills                []SkillState
	AvailableModels       []string
}

func (s SessionState) Ref() SessionRef {
	return SessionRef{AgentID: s.AgentID, SessionID: s.ID}
}

// State owns desktop project and session state independently from UI widgets.
type State struct {
	ActiveSessionID string
	ActiveAgentID   string
	ActiveProjectID string
	Projects        []ProjectState
	Sessions        []SessionState
	PermissionInbox []PermissionRequest
	QuestionInbox   []QuestionRequest
	Integrations    []MCPIntegrationState
	Providers       []ProviderState
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
	EventQuestionRequested
	EventQuestionResolved
	EventTimelineAppended
	EventTimelineUpserted
	EventSubagentUpserted
	EventSessionContextUpdated
	EventSessionMemoryUpdated
	EventSessionRuntimeUpdated
	EventIntegrationsReplaced
	EventSessionSkillsUpdated
	EventProvidersUpdated
)

// Event is a typed reducer input. Only fields relevant to Kind are consumed.
type Event struct {
	Kind            EventKind
	SessionID       string
	AgentID         string
	Sessions        []SessionState
	Item            TimelineItem
	Subagent        SubagentState
	Context         SessionContextState
	Memory          MemoryState
	Runtime         RuntimeSettingsState
	Skills          []SkillState
	Integrations    []MCPIntegrationState
	Permission      PermissionRequest
	Question        QuestionRequest
	RequestID       string
	AvailableModels []string
	Providers       []ProviderState
}

// Reduce applies one event and returns a new state without aliasing caller-owned slices.
func Reduce(current State, event Event) State {
	next := cloneState(current)
	applyEvent(&next, event)
	return next
}

// Apply applies one event to state owned by the caller. It avoids cloning the
// complete desktop snapshot on hot streaming paths while still copying slices
// supplied by the event before retaining them.
func Apply(state *State, event Event) {
	if state == nil {
		return
	}
	applyEvent(state, event)
}

func applyEvent(state *State, event Event) {
	switch event.Kind {
	case EventSessionsReplaced:
		state.Sessions = cloneSessions(event.Sessions)
		if state.ActiveSessionID != "" && !hasSession(state.Sessions, state.ActiveSessionID, state.ActiveAgentID) {
			state.ActiveSessionID = ""
			state.ActiveAgentID = ""
		}
	case EventSessionSelected:
		if hasSession(state.Sessions, event.SessionID, event.AgentID) {
			state.ActiveSessionID = event.SessionID
			state.ActiveAgentID = event.AgentID
			if state.ActiveAgentID == "" {
				if session := sessionByID(state, event.SessionID, ""); session != nil {
					state.ActiveAgentID = session.AgentID
				}
			}
		}
	case EventPromptQueued:
		setStatus(state, event.AgentID, event.SessionID, TaskQueued)
	case EventPromptStarted:
		setStatus(state, event.AgentID, event.SessionID, TaskRunning)
	case EventPromptCompleted:
		setStatus(state, event.AgentID, event.SessionID, TaskCompleted)
	case EventPromptFailed:
		setStatus(state, event.AgentID, event.SessionID, TaskFailed)
	case EventPermissionRequested:
		setStatus(state, event.AgentID, event.SessionID, TaskWaitingPermission)
		if event.Permission.RequestID != "" && !hasPermission(state.PermissionInbox, event.Permission.RequestID, event.AgentID) {
			state.PermissionInbox = append(state.PermissionInbox, clonePermission(event.Permission))
		}
	case EventPermissionResolved:
		setStatus(state, event.AgentID, event.SessionID, TaskRunning)
		state.PermissionInbox = removePermission(state.PermissionInbox, event.RequestID, event.AgentID)
	case EventQuestionRequested:
		setStatus(state, event.AgentID, event.SessionID, TaskWaitingUser)
		if event.Question.RequestID != "" && !hasQuestion(state.QuestionInbox, event.Question.RequestID, event.AgentID) {
			state.QuestionInbox = append(state.QuestionInbox, cloneQuestion(event.Question))
		}
	case EventQuestionResolved:
		setStatus(state, event.AgentID, event.SessionID, TaskRunning)
		state.QuestionInbox = removeQuestion(state.QuestionInbox, event.RequestID, event.AgentID)
	case EventTimelineAppended:
		if session := sessionByID(state, event.SessionID, event.AgentID); session != nil {
			session.Timeline = append(session.Timeline, event.Item)
		}
	case EventTimelineUpserted:
		if session := sessionByID(state, event.SessionID, event.AgentID); session != nil {
			upsertTimeline(session, event.Item)
		}
	case EventSubagentUpserted:
		if session := sessionByID(state, event.SessionID, event.AgentID); session != nil {
			upsertSubagent(session, event.Subagent)
		}
	case EventSessionContextUpdated:
		if session := sessionByID(state, event.SessionID, event.AgentID); session != nil {
			session.Context.Goal = event.Context.Goal
			session.Context.Todo = cloneTodoState(event.Context.Todo)
		}
	case EventSessionMemoryUpdated:
		if session := sessionByID(state, event.SessionID, event.AgentID); session != nil {
			session.Context.Memory = cloneMemoryState(event.Memory)
		}
	case EventSessionRuntimeUpdated:
		if session := sessionByID(state, event.SessionID, event.AgentID); session != nil {
			session.Runtime = event.Runtime
			if len(event.AvailableModels) > 0 {
				session.AvailableModels = slices.Clone(event.AvailableModels)
			}
		}
	case EventIntegrationsReplaced:
		state.Integrations = cloneIntegrations(event.Integrations)
	case EventSessionSkillsUpdated:
		if session := sessionByID(state, event.SessionID, event.AgentID); session != nil {
			session.Skills = cloneSkillStates(event.Skills)
		}
	case EventProvidersUpdated:
		state.Providers = cloneProviders(event.Providers)
	}
}

func cloneState(state State) State {
	state.Projects = cloneProjects(state.Projects)
	state.Sessions = cloneSessions(state.Sessions)
	state.PermissionInbox = clonePermissions(state.PermissionInbox)
	state.QuestionInbox = cloneQuestions(state.QuestionInbox)
	state.Integrations = cloneIntegrations(state.Integrations)
	state.Providers = cloneProviders(state.Providers)
	return state
}

func CloneState(state State) State {
	return cloneState(state)
}

// ClonePresentationState returns an immutable-enough view for the desktop
// renderer. The active session is fully detached because its timeline and
// context are rendered; inactive sessions retain only the metadata needed by
// navigation, avoiding a full-state copy on every streamed update.
func ClonePresentationState(state State) State {
	next := state
	next.Projects = cloneProjects(state.Projects)
	next.Sessions = cloneSessionMetadata(state.Sessions, state.ActiveSessionID, state.ActiveAgentID)
	next.PermissionInbox = clonePermissions(state.PermissionInbox)
	next.QuestionInbox = cloneQuestions(state.QuestionInbox)
	next.Integrations = cloneIntegrations(state.Integrations)
	next.Providers = cloneProviders(state.Providers)
	return next
}

func cloneProviders(items []ProviderState) []ProviderState {
	return slices.Clone(items)
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
		out[i] = cloneSession(out[i])
	}
	return out
}

func cloneSession(session SessionState) SessionState {
	session.Timeline = slices.Clone(session.Timeline)
	session.Subagents = slices.Clone(session.Subagents)
	session.AdditionalDirectories = slices.Clone(session.AdditionalDirectories)
	session.Context = cloneSessionContext(session.Context)
	session.Skills = cloneSkillStates(session.Skills)
	session.AvailableModels = slices.Clone(session.AvailableModels)
	return session
}

func cloneSkillStates(skills []SkillState) []SkillState {
	if skills == nil {
		return nil
	}
	out := slices.Clone(skills)
	for i := range out {
		out[i].Resources = slices.Clone(out[i].Resources)
	}
	return out
}

func cloneSessionMetadata(sessions []SessionState, activeSessionID, activeAgentID string) []SessionState {
	out := slices.Clone(sessions)
	for index := range out {
		session := out[index]
		if session.ID == activeSessionID && (activeAgentID == "" || session.AgentID == activeAgentID) {
			out[index] = cloneSession(session)
			continue
		}
		out[index] = SessionState{
			ID:               session.ID,
			AgentID:          session.AgentID,
			ProjectID:        session.ProjectID,
			Title:            session.Title,
			Workspace:        session.Workspace,
			WorkspaceKey:     session.WorkspaceKey,
			WorkspaceName:    session.WorkspaceName,
			LastActivityAt:   session.LastActivityAt,
			Status:           session.Status,
			HistoryTruncated: session.HistoryTruncated,
		}
	}
	return out
}

func cloneSessionContext(context SessionContextState) SessionContextState {
	context.Todo = cloneTodoState(context.Todo)
	context.Memory = cloneMemoryState(context.Memory)
	return context
}

func cloneTodoState(todo TodoState) TodoState {
	todo.Items = slices.Clone(todo.Items)
	return todo
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

func hasSession(sessions []SessionState, id string, agentIDs ...string) bool {
	for i := range sessions {
		if sessions[i].ID == id && (len(agentIDs) == 0 || agentIDs[0] == "" || sessions[i].AgentID == agentIDs[0]) {
			return true
		}
	}
	return false
}

func hasPermission(items []PermissionRequest, id string, agentIDs ...string) bool {
	for i := range items {
		if items[i].RequestID == id && (len(agentIDs) == 0 || agentIDs[0] == "" || items[i].AgentID == agentIDs[0]) {
			return true
		}
	}
	return false
}

func removePermission(items []PermissionRequest, id string, agentIDs ...string) []PermissionRequest {
	for i := range items {
		if items[i].RequestID == id && (len(agentIDs) == 0 || agentIDs[0] == "" || items[i].AgentID == agentIDs[0]) {
			copy(items[i:], items[i+1:])
			items[len(items)-1] = PermissionRequest{}
			return items[:len(items)-1]
		}
	}
	return items
}

func sessionByID(state *State, id string, agentIDs ...string) *SessionState {
	for i := range state.Sessions {
		if state.Sessions[i].ID == id && (len(agentIDs) == 0 || agentIDs[0] == "" || state.Sessions[i].AgentID == agentIDs[0]) {
			return &state.Sessions[i]
		}
	}
	return nil
}

func setStatus(state *State, agentID, id string, status TaskStatus) {
	if session := sessionByID(state, id, agentID); session != nil {
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

func hasQuestion(items []QuestionRequest, id string, agentIDs ...string) bool {
	for i := range items {
		if items[i].RequestID == id && (len(agentIDs) == 0 || agentIDs[0] == "" || items[i].AgentID == agentIDs[0]) {
			return true
		}
	}
	return false
}

func removeQuestion(items []QuestionRequest, id string, agentIDs ...string) []QuestionRequest {
	for i := range items {
		if items[i].RequestID == id && (len(agentIDs) == 0 || agentIDs[0] == "" || items[i].AgentID == agentIDs[0]) {
			copy(items[i:], items[i+1:])
			items[len(items)-1] = QuestionRequest{}
			return items[:len(items)-1]
		}
	}
	return items
}

func cloneQuestions(items []QuestionRequest) []QuestionRequest {
	if len(items) == 0 {
		return nil
	}
	out := make([]QuestionRequest, len(items))
	for i, q := range items {
		out[i] = cloneQuestion(q)
	}
	return out
}

func cloneQuestion(q QuestionRequest) QuestionRequest {
	clone := q
	if len(q.Questions) > 0 {
		clone.Questions = make([]QuestionItemState, len(q.Questions))
		for i, item := range q.Questions {
			clone.Questions[i] = QuestionItemState{
				Question:    item.Question,
				Options:     append([]string(nil), item.Options...),
				Multiple:    item.Multiple,
				Recommended: item.Recommended,
			}
		}
	}
	return clone
}
