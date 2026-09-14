package acp

import (
	"context"
	"fmt"
	"strings"

	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

const methodSessionContext = "protonman/session/context"

// ProtonmanSessionContextParams identifies the session whose durable working
// context should be inspected by a native Protonman client.
type ProtonmanSessionContextParams struct {
	SessionID string `json:"sessionId"`
}

// ProtonmanTodoItem is the ACP-safe projection of one durable TODO item.
type ProtonmanTodoItem struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`
}

// ProtonmanTodoSnapshot is the revisioned TODO projection exposed to clients.
type ProtonmanTodoSnapshot struct {
	Revision uint64              `json:"revision"`
	Items    []ProtonmanTodoItem `json:"items"`
}

// ProtonmanSessionContextResult is the typed, read-only inspector payload for
// the active goal and durable TODO state. Mutations deliberately use separate
// methods so inspecting state can never accidentally execute work.
type ProtonmanSessionContextResult struct {
	SessionID string                `json:"sessionId"`
	Goal      string                `json:"goal,omitempty"`
	Todo      ProtonmanTodoSnapshot `json:"todo"`
}

func (s *Server) sessionContext(ctx context.Context, sessionID string) (ProtonmanSessionContextResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ProtonmanSessionContextResult{}, fmt.Errorf("sessionId is required")
	}
	if s.sessionService == nil {
		return ProtonmanSessionContextResult{}, fmt.Errorf("session persistence is unavailable")
	}

	goal := ""
	detail, err := s.sessionService.LoadDetail(ctx, sessionID, "")
	if err != nil {
		if _, active := s.lookupSession(sessionID); !active {
			return ProtonmanSessionContextResult{}, err
		}
	} else {
		goal = strings.TrimSpace(detail.ActiveGoal)
	}

	store, err := s.sessionService.OpenTodoStore(ctx, sessionID, goal)
	if err != nil {
		return ProtonmanSessionContextResult{}, fmt.Errorf("open todo state for session %q: %w", sessionID, err)
	}
	return ProtonmanSessionContextResult{
		SessionID: sessionID,
		Goal:      goal,
		Todo:      projectTodoSnapshot(store.Snapshot()),
	}, nil
}

func projectTodoSnapshot(snapshot tododomain.Snapshot) ProtonmanTodoSnapshot {
	items := make([]ProtonmanTodoItem, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		items = append(items, ProtonmanTodoItem{
			ID:     item.ID,
			Text:   item.Text,
			Status: string(item.Status),
		})
	}
	return ProtonmanTodoSnapshot{Revision: snapshot.Revision, Items: items}
}
