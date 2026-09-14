package acp

import (
	"context"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/app"
)

const methodSessionMemory = "protonman/session/memory"

type ProtonmanSessionMemoryParams struct {
	SessionID string `json:"sessionId"`
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

func (s *Server) sessionMemory(ctx context.Context, sessionID string) (ProtonmanSessionMemoryResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ProtonmanSessionMemoryResult{}, fmt.Errorf("sessionId is required")
	}
	if s.memories == nil {
		return ProtonmanSessionMemoryResult{}, fmt.Errorf("memory inspection is unavailable")
	}
	workspaceKey := ""
	if sess, ok := s.lookupSession(sessionID); ok {
		workspaceKey = strings.TrimSpace(sess.workspaceKey)
	}
	if workspaceKey == "" && s.sessionService != nil {
		detail, err := s.sessionService.LoadDetail(ctx, sessionID, "")
		if err != nil {
			return ProtonmanSessionMemoryResult{}, err
		}
		workspaceKey = strings.TrimSpace(detail.WorkspaceKey)
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
