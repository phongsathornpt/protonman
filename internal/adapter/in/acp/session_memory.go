package acp

import (
	"context"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/app"
)

const methodSessionMemory = "protonman/session/memory"

// methodSessionMemoryForget is the correction capability for durable memory.
// It is separate from inspection because forgetting changes future model
// behavior, whereas inspection is side-effect free.
const methodSessionMemoryForget = "protonman/session/memory/forget"

type ProtonmanSessionMemoryParams struct {
	SessionID string `json:"sessionId"`
}

// ProtonmanSessionMemoryForgetParams carries only the session, the target scope,
// and the entry ids. The workspace key is resolved server-side from the session
// so a client cannot direct a forget at an unrelated workspace.
type ProtonmanSessionMemoryForgetParams struct {
	SessionID string   `json:"sessionId"`
	Scope     string   `json:"scope,omitempty"`
	IDs       []string `json:"ids"`
}

type ProtonmanSessionMemoryForgetResult struct {
	SessionID string `json:"sessionId"`
	Scope     string `json:"scope"`
	Removed   int    `json:"removed"`
}

type ProtonmanMemoryEntry struct {
	ID         string  `json:"id"`
	Scope      string  `json:"scope"`
	Kind       string  `json:"kind"`
	Key        string  `json:"key"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	UsageCount uint64  `json:"usageCount,omitempty"`
}

type ProtonmanSessionMemoryResult struct {
	SessionID    string                 `json:"sessionId"`
	WorkspaceKey string                 `json:"workspaceKey,omitempty"`
	Workspace    []ProtonmanMemoryEntry `json:"workspace"`
	Global       []ProtonmanMemoryEntry `json:"global"`
}

// sessionWorkspaceKey resolves the workspace bound to a session from trusted
// server-side state. Callers must never accept a workspace key from a client
// payload, because that would let one session forget another workspace's memory.
func (s *Server) sessionWorkspaceKey(ctx context.Context, sessionID string) (string, error) {
	if sess, ok := s.lookupSession(sessionID); ok {
		if workspaceKey := strings.TrimSpace(sess.workspaceKey); workspaceKey != "" {
			return workspaceKey, nil
		}
	}
	if s.sessionService != nil {
		detail, err := s.sessionService.LoadDetail(ctx, sessionID, "")
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(detail.WorkspaceKey), nil
	}
	return "", nil
}

func (s *Server) sessionMemory(ctx context.Context, sessionID string) (ProtonmanSessionMemoryResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ProtonmanSessionMemoryResult{}, fmt.Errorf("sessionId is required")
	}
	if s.memories == nil {
		return ProtonmanSessionMemoryResult{}, fmt.Errorf("memory inspection is unavailable")
	}
	workspaceKey, err := s.sessionWorkspaceKey(ctx, sessionID)
	if err != nil {
		return ProtonmanSessionMemoryResult{}, err
	}
	snapshot, err := s.memories.Inspect(ctx, workspaceKey)
	if err != nil {
		return ProtonmanSessionMemoryResult{}, err
	}
	return ProtonmanSessionMemoryResult{
		SessionID:    sessionID,
		WorkspaceKey: snapshot.WorkspaceKey,
		Workspace:    projectMemoryEntries(snapshot.Workspace),
		Global:       projectMemoryEntries(snapshot.Global),
	}, nil
}

func (s *Server) sessionMemoryForget(ctx context.Context, params ProtonmanSessionMemoryForgetParams) (ProtonmanSessionMemoryForgetResult, error) {
	sessionID := strings.TrimSpace(params.SessionID)
	if sessionID == "" {
		return ProtonmanSessionMemoryForgetResult{}, fmt.Errorf("sessionId is required")
	}
	if s.memories == nil {
		return ProtonmanSessionMemoryForgetResult{}, fmt.Errorf("memory inspection is unavailable")
	}
	scope := app.MemoryScope(strings.ToLower(strings.TrimSpace(params.Scope)))
	if scope == "" {
		// Default to the session's own workspace: the narrowest useful scope.
		scope = app.MemoryScopeWorkspace
	}
	request := app.MemoryForgetRequest{Scope: scope, IDs: params.IDs}
	if scope == app.MemoryScopeWorkspace {
		workspaceKey, err := s.sessionWorkspaceKey(ctx, sessionID)
		if err != nil {
			return ProtonmanSessionMemoryForgetResult{}, err
		}
		if workspaceKey == "" {
			return ProtonmanSessionMemoryForgetResult{}, fmt.Errorf("session %q is not bound to a workspace", sessionID)
		}
		request.WorkspaceKey = workspaceKey
	}
	result, err := s.memories.Forget(ctx, request)
	if err != nil {
		return ProtonmanSessionMemoryForgetResult{}, err
	}
	return ProtonmanSessionMemoryForgetResult{
		SessionID: sessionID,
		Scope:     string(result.Scope),
		Removed:   result.Removed,
	}, nil
}

func projectMemoryEntries(entries []app.MemoryEntry) []ProtonmanMemoryEntry {
	out := make([]ProtonmanMemoryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, ProtonmanMemoryEntry{
			ID:         entry.ID,
			Scope:      entry.Scope,
			Kind:       entry.Kind,
			Key:        entry.Key,
			Value:      entry.Value,
			Confidence: entry.Confidence,
			UsageCount: entry.UsageCount,
		})
	}
	return out
}
