package acp

import (
	"context"
	"fmt"
	"strings"

	coretodo "github.com/phongsathornpt/protonman/internal/core/todo"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

const methodSessionContext = "protonman/session/context"
const methodSessionTodoUpdate = "protonman/session/todo/update"
const methodSessionTodoPatch = "protonman/session/todo/patch"

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

type ProtonmanSessionTodoUpdateParams struct {
	SessionID string          `json:"sessionId"`
	Revision  uint64          `json:"revision"`
	ItemID    string          `json:"itemId"`
	Status    coretodo.Status `json:"status"`
}

type ProtonmanSessionTodoUpdateResult struct {
	SessionID string                `json:"sessionId"`
	Todo      ProtonmanTodoSnapshot `json:"todo"`
}

type ProtonmanSessionTodoPatchParams struct {
	SessionID  string               `json:"sessionId"`
	Revision   uint64               `json:"revision"`
	Operations []coretodo.Operation `json:"operations"`
}

type ProtonmanSessionTodoPatchResult = ProtonmanSessionTodoUpdateResult

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

func (s *Server) sessionTodoUpdate(ctx context.Context, params ProtonmanSessionTodoUpdateParams) (ProtonmanSessionTodoUpdateResult, error) {
	sessionID := strings.TrimSpace(params.SessionID)
	if sessionID == "" {
		return ProtonmanSessionTodoUpdateResult{}, fmt.Errorf("sessionId is required")
	}
	itemID := strings.TrimSpace(params.ItemID)
	if itemID == "" {
		return ProtonmanSessionTodoUpdateResult{}, fmt.Errorf("itemId is required")
	}
	if !params.Status.Valid() {
		return ProtonmanSessionTodoUpdateResult{}, fmt.Errorf("invalid todo status %q", params.Status)
	}
	if s.sessionService == nil {
		return ProtonmanSessionTodoUpdateResult{}, fmt.Errorf("session persistence is unavailable")
	}
	return s.sessionTodoPatch(ctx, ProtonmanSessionTodoPatchParams{
		SessionID: sessionID,
		Revision:  params.Revision,
		Operations: []coretodo.Operation{{
			Op:     coretodo.PatchSetStatus,
			ID:     itemID,
			Status: params.Status,
		}},
	})
}

func (s *Server) sessionTodoPatch(ctx context.Context, params ProtonmanSessionTodoPatchParams) (ProtonmanSessionTodoPatchResult, error) {
	sessionID := strings.TrimSpace(params.SessionID)
	if sessionID == "" {
		return ProtonmanSessionTodoPatchResult{}, fmt.Errorf("sessionId is required")
	}
	if len(params.Operations) == 0 {
		return ProtonmanSessionTodoPatchResult{}, fmt.Errorf("todo patch requires at least one operation")
	}
	if s.sessionService == nil {
		return ProtonmanSessionTodoPatchResult{}, fmt.Errorf("session persistence is unavailable")
	}
	goal := ""
	detail, err := s.sessionService.LoadDetail(ctx, sessionID, "")
	if err != nil {
		if _, active := s.lookupSession(sessionID); !active {
			return ProtonmanSessionTodoPatchResult{}, err
		}
	} else {
		goal = strings.TrimSpace(detail.ActiveGoal)
	}
	store, err := s.sessionService.OpenTodoStore(ctx, sessionID, goal)
	if err != nil {
		return ProtonmanSessionTodoPatchResult{}, fmt.Errorf("open todo state for session %q: %w", sessionID, err)
	}
	patchStore, ok := store.(coretodo.PatchRepository)
	if !ok {
		return ProtonmanSessionTodoPatchResult{}, fmt.Errorf("todo updates are unavailable")
	}
	_, after, err := patchStore.CompareAndPatch(ctx, params.Revision, params.Operations)
	if err != nil {
		return ProtonmanSessionTodoPatchResult{}, err
	}
	return ProtonmanSessionTodoPatchResult{SessionID: sessionID, Todo: projectTodoSnapshot(after)}, nil
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
