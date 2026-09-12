package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/core/session"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Sessions exposes persisted-session use cases to inbound adapters.
type Sessions struct {
	repository   session.Repository
	sessionsRoot string
}

func NewSessions(repository session.Repository) *Sessions {
	if repository == nil {
		return nil
	}
	return &Sessions{repository: repository}
}

// WithSessionsRoot configures the filesystem root for session resource resolution.
func (s *Sessions) WithSessionsRoot(root string) *Sessions {
	if s == nil {
		return nil
	}
	s.sessionsRoot = root
	return s
}

type SessionListOptions struct {
	WorkspaceKey string
	Prefix       string
	Limit        int
	Offset       int
}

type SessionSummary = session.Summary

// SessionDetail provides adapter-safe session state with provider-neutral messages.
type SessionDetail struct {
	ID                 string
	Revision           uint64
	WorkspaceKey       string
	WorkspaceName      string
	PermissionMode     string
	ActiveSkills       []string
	ActiveGoal         string
	AgentProfile       string
	ReasoningEffort    string
	LowConcurrencyMode string
	Messages           []sdk.Message
	UpdatedAt          time.Time
}

func (s *Sessions) ListSummaries(ctx context.Context, options SessionListOptions) ([]SessionSummary, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("session repository is unavailable")
	}
	return s.repository.ListSummaries(ctx, session.ListOptions{
		WorkspaceKey: options.WorkspaceKey,
		Prefix:       options.Prefix,
		Limit:        options.Limit,
		Offset:       options.Offset,
	})
}

func (s *Sessions) Load(ctx context.Context, id string) (session.State, bool, error) {
	if s == nil || s.repository == nil {
		return session.State{}, false, fmt.Errorf("session repository is unavailable")
	}
	return s.repository.Load(ctx, id)
}

// LoadDetail loads a session by ID and validates that it matches the expected workspace.
func (s *Sessions) LoadDetail(ctx context.Context, id, expectedWorkspaceKey string) (*SessionDetail, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("session repository is unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("session id cannot be empty")
	}
	state, found, err := s.repository.Load(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load session %q: %w", id, err)
	}
	if !found {
		return nil, fmt.Errorf("session %q not found", id)
	}
	if expectedWorkspaceKey != "" && state.WorkspaceKey != "" && state.WorkspaceKey != expectedWorkspaceKey {
		return nil, fmt.Errorf("session %q belongs to another workspace", id)
	}
	return stateToDetail(state), nil
}

// LatestDetail finds the most recent session for the given workspace key.
func (s *Sessions) LatestDetail(ctx context.Context, workspaceKey string) (*SessionDetail, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("session repository is unavailable")
	}
	wsPrefix := "workspace-" + strings.TrimSpace(workspaceKey)
	id, state, found, err := s.repository.LatestSession(ctx, wsPrefix)
	if err != nil {
		return nil, fmt.Errorf("find latest session: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("no previous session found for workspace")
	}
	if id != "" {
		state.SessionID = id
	}
	return stateToDetail(state), nil
}

// SaveCurrent persists the given session detail snapshot.
func (s *Sessions) SaveCurrent(ctx context.Context, detail SessionDetail) error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("session repository is unavailable")
	}
	id := strings.TrimSpace(detail.ID)
	if id == "" {
		return fmt.Errorf("session id cannot be empty")
	}
	state := session.State{
		SessionID:          id,
		Revision:           detail.Revision,
		WorkspaceKey:       detail.WorkspaceKey,
		WorkspaceName:      detail.WorkspaceName,
		PermissionMode:     detail.PermissionMode,
		ActiveSkills:       append([]string(nil), detail.ActiveSkills...),
		ActiveGoal:         detail.ActiveGoal,
		AgentProfile:       detail.AgentProfile,
		ReasoningEffort:    detail.ReasoningEffort,
		LowConcurrencyMode: detail.LowConcurrencyMode,
		Messages:           session.FromModelMessages(detail.Messages),
		UpdatedAt:          time.Now().UTC(),
	}
	return s.repository.Save(ctx, id, state)
}

// OpenTodoStore opens the session-scoped TODO markdown store and binds the active goal.
func (s *Sessions) OpenTodoStore(ctx context.Context, sessionID, activeGoal string) (tododomain.Repository, error) {
	root := s.sessionsRoot
	if root == "" {
		dirs, err := appdirs.Resolve("")
		if err != nil {
			return nil, fmt.Errorf("resolve session store: %w", err)
		}
		root = dirs.Sessions
	}
	resources, err := session.ResolveResources(root, sessionID)
	if err != nil {
		return nil, fmt.Errorf("resolve session resources: %w", err)
	}
	store, err := tododomain.OpenMarkdownStore(ctx, resources.Todo)
	if err != nil {
		return nil, fmt.Errorf("open session todo store: %w", err)
	}
	if _, _, err := store.BindGoal(ctx, activeGoal); err != nil {
		return nil, fmt.Errorf("bind session todo store to goal: %w", err)
	}
	return store, nil
}

func (s *Sessions) Save(ctx context.Context, id string, state session.State) error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("session repository is unavailable")
	}
	return s.repository.Save(ctx, id, state)
}

func (s *Sessions) Delete(ctx context.Context, id string) error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("session repository is unavailable")
	}
	return s.repository.Delete(ctx, id)
}

func (s *Sessions) List(ctx context.Context, prefix string) ([]string, error) {
	if s == nil || s.repository == nil {
		return nil, fmt.Errorf("session repository is unavailable")
	}
	return s.repository.List(ctx, prefix)
}

func stateToDetail(state session.State) *SessionDetail {
	return &SessionDetail{
		ID:                 state.SessionID,
		Revision:           state.Revision,
		WorkspaceKey:       state.WorkspaceKey,
		WorkspaceName:      state.WorkspaceName,
		PermissionMode:     state.PermissionMode,
		ActiveSkills:       append([]string(nil), state.ActiveSkills...),
		ActiveGoal:         state.ActiveGoal,
		AgentProfile:       state.AgentProfile,
		ReasoningEffort:    state.ReasoningEffort,
		LowConcurrencyMode: state.LowConcurrencyMode,
		Messages:           session.ToModelMessages(state.Messages),
		UpdatedAt:          state.UpdatedAt,
	}
}

type memorySessionRepository struct {
	states map[string]session.State
}

// NewMemorySessions returns an in-memory Sessions service for testing and isolated runtimes.
func NewMemorySessions() *Sessions {
	return NewSessions(&memorySessionRepository{
		states: make(map[string]session.State),
	})
}

func (m *memorySessionRepository) Load(_ context.Context, id string) (session.State, bool, error) {
	st, ok := m.states[id]
	return st, ok, nil
}

func (m *memorySessionRepository) LatestSession(_ context.Context, prefix string) (string, session.State, bool, error) {
	var latestID string
	var latestState session.State
	var latestTime time.Time
	for id, st := range m.states {
		if prefix != "" && !strings.HasPrefix(id, prefix) {
			continue
		}
		if st.UpdatedAt.After(latestTime) || latestID == "" {
			latestID = id
			latestState = st
			latestTime = st.UpdatedAt
		}
	}
	if latestID == "" {
		return "", session.State{}, false, nil
	}
	return latestID, latestState, true, nil
}

func (m *memorySessionRepository) Save(_ context.Context, id string, state session.State) error {
	m.states[id] = state
	return nil
}

func (m *memorySessionRepository) Delete(_ context.Context, id string) error {
	delete(m.states, id)
	return nil
}

func (m *memorySessionRepository) List(_ context.Context, prefix string) ([]string, error) {
	var out []string
	for id := range m.states {
		if prefix == "" || strings.HasPrefix(id, prefix) {
			out = append(out, id)
		}
	}
	return out, nil
}

func (m *memorySessionRepository) ListSummaries(_ context.Context, options session.ListOptions) ([]session.Summary, error) {
	var out []session.Summary
	for id, st := range m.states {
		if options.WorkspaceKey != "" && st.WorkspaceKey != "" && st.WorkspaceKey != options.WorkspaceKey {
			continue
		}
		if options.Prefix != "" && !strings.HasPrefix(id, options.Prefix) {
			continue
		}
		preview := ""
		if len(st.Messages) > 0 {
			preview = st.Messages[len(st.Messages)-1].Content
		}
		out = append(out, session.Summary{
			ID:                 id,
			WorkspaceKey:       st.WorkspaceKey,
			WorkspaceName:      st.WorkspaceName,
			CreatedAt:          st.CreatedAt,
			UpdatedAt:          st.UpdatedAt,
			AgentProfile:       st.AgentProfile,
			ReasoningEffort:    st.ReasoningEffort,
			LowConcurrencyMode: st.LowConcurrencyMode,
			MessageCount:       len(st.Messages),
			Preview:            preview,
		})
	}
	return out, nil
}
