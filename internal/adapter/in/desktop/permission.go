//go:build desktop

package desktop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const requestPermissionMethod = "session/request_permission"

type permissionParams struct {
	SessionID string `json:"sessionId"`
	ToolCall  map[string]any `json:"toolCall"`
	Options   []struct {
		OptionID string `json:"optionId"`
		Name string `json:"name"`
		Kind string `json:"kind"`
	} `json:"options"`
}

func (a *application) handleRequest(ctx context.Context, request acpclient.Request) (any, error) {
	if request.Method != requestPermissionMethod {
		return nil, fmt.Errorf("%w: %s", acpclient.ErrMethodNotHandled, request.Method)
	}
	var params permissionParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return nil, fmt.Errorf("decode permission request: %w", err)
	}
	requestID := string(request.ID)
	item := desktopstate.PermissionRequest{RequestID: requestID, SessionID: params.SessionID, Title: "Tool permission"}
	if title, ok := params.ToolCall["title"].(string); ok && title != "" {
		item.Title = title
	}
	for _, option := range params.Options {
		item.Options = append(item.Options, desktopstate.PermissionOption{ID: option.OptionID, Name: option.Name, Kind: option.Kind})
	}
	waiter := make(chan string, 1)
	a.mu.Lock()
	if a.permissionWaiters == nil {
		a.permissionWaiters = make(map[string]chan string)
	}
	a.permissionWaiters[requestID] = waiter
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventPermissionRequested, SessionID: params.SessionID, Permission: item})
	a.mu.Unlock()
	a.refreshPermissionView()

	select {
	case <-ctx.Done():
		a.finishPermission(requestID, params.SessionID)
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}, nil
	case optionID := <-waiter:
		a.finishPermission(requestID, params.SessionID)
		return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}, nil
	}
}

func (a *application) finishPermission(requestID, sessionID string) {
	a.mu.Lock()
	delete(a.permissionWaiters, requestID)
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventPermissionResolved, SessionID: sessionID, RequestID: requestID})
	a.mu.Unlock()
	a.refreshPermissionView()
}

func (a *application) resolvePermission(requestID, optionID string) {
	a.mu.Lock()
	waiter := a.permissionWaiters[requestID]
	a.mu.Unlock()
	if waiter == nil {
		return
	}
	select {
	case waiter <- optionID:
	default:
	}
}
