//go:build desktop

package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const requestPermissionMethod = "session/request_permission"

const maxPermissionDetailBytes = 4096

type permissionParams struct {
	SessionID string         `json:"sessionId"`
	ToolCall  map[string]any `json:"toolCall"`
	Options   []struct {
		OptionID string `json:"optionId"`
		Name     string `json:"name"`
		Kind     string `json:"kind"`
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
	item := desktopstate.PermissionRequest{
		RequestID: requestID,
		SessionID: params.SessionID,
		Title:     "Tool permission",
		Detail:    permissionDetail(params.ToolCall),
	}
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
	a.notifyPermission(item)

	select {
	case <-ctx.Done():
		a.finishPermission(requestID, params.SessionID, waiter)
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}, nil
	case optionID := <-waiter:
		a.finishPermission(requestID, params.SessionID, waiter)
		return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}, nil
	}
}

func permissionDetail(toolCall map[string]any) string {
	if len(toolCall) == 0 {
		return "Tool details were not provided by the runtime."
	}
	payload, err := json.MarshalIndent(toolCall, "", "  ")
	if err != nil {
		return "Tool details could not be rendered: " + err.Error()
	}
	text := strings.TrimSpace(string(payload))
	if len(text) > maxPermissionDetailBytes {
		text = text[:maxPermissionDetailBytes]
		for len(text) > 0 && !strings.HasSuffix(text, "\n") && len(text) > maxPermissionDetailBytes-256 {
			text = text[:len(text)-1]
		}
		text = strings.TrimSpace(text) + "\n… truncated"
	}
	return "Tool request:\n" + text
}

func (a *application) finishPermission(requestID, sessionID string, waiter chan string) {
	if !a.finishPermissionState(requestID, sessionID, waiter) {
		return
	}
	a.refreshPermissionView()
}

func (a *application) finishPermissionState(requestID, sessionID string, waiter chan string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.permissionWaiters[requestID] != waiter {
		return false
	}
	delete(a.permissionWaiters, requestID)
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventPermissionResolved, SessionID: sessionID, RequestID: requestID})
	return true
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
