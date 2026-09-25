//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	requestPermissionMethod  = "session/request_permission"
	maxPermissionDetailBytes = 4096
	permissionDetailPrefix   = "Tool request:\n"
	permissionTruncationMark = "\n… truncated"
)

type permissionParams struct {
	SessionID string         `json:"sessionId"`
	ToolCall  map[string]any `json:"toolCall"`
	Options   []struct {
		OptionID string `json:"optionId"`
		Name     string `json:"name"`
		Kind     string `json:"kind"`
	} `json:"options"`
}

func (c *controller) handlePermissionRequest(ctx context.Context, request acpclient.Request) (any, error) {
	return c.handlePermissionRequestFrom(nil, ctx, request)
}

func (c *controller) handlePermissionRequestFor(client *acpclient.Client, ctx context.Context, request acpclient.Request) (any, error) {
	return c.handlePermissionRequestFrom(client, ctx, request)
}

func (c *controller) handlePermissionRequestForAgent(agentID string, client *acpclient.Client, ctx context.Context, request acpclient.Request) (any, error) {
	return c.handlePermissionRequestFromAgent(agentID, client, ctx, request)
}

func (c *controller) handlePermissionRequestFrom(source *acpclient.Client, ctx context.Context, request acpclient.Request) (any, error) {
	return c.handlePermissionRequestFromAgent("", source, ctx, request)
}

func (c *controller) handlePermissionRequestFromAgent(agentID string, source *acpclient.Client, ctx context.Context, request acpclient.Request) (any, error) {
	if request.Method != requestPermissionMethod {
		return nil, fmt.Errorf("%w: %s", acpclient.ErrMethodNotHandled, request.Method)
	}
	var params permissionParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return nil, fmt.Errorf("decode permission request: %w", err)
	}
	if strings.TrimSpace(params.SessionID) == "" || len(params.Options) == 0 {
		return nil, errors.New("permission request must include a session ID and options")
	}
	item := desktopstate.PermissionRequest{
		RequestID: string(request.ID),
		SessionID: params.SessionID,
		Title:     "Tool permission",
		Detail:    permissionDetail(params.ToolCall),
	}
	if item.RequestID == "" {
		return nil, errors.New("permission request must include an ID")
	}
	if title, ok := params.ToolCall["title"].(string); ok && strings.TrimSpace(title) != "" {
		item.Title = strings.TrimSpace(title)
	}
	for _, option := range params.Options {
		optionID := strings.TrimSpace(option.OptionID)
		if optionID == "" {
			return nil, errors.New("permission option must include an ID")
		}
		name := strings.TrimSpace(option.Name)
		if name == "" {
			name = optionID
		}
		item.Options = append(item.Options, desktopstate.PermissionOption{ID: optionID, Name: name, Kind: strings.TrimSpace(option.Kind)})
	}

	waiter := make(chan string, 1)
	c.mu.Lock()
	if source != nil {
		currentAgentID := c.agentIDForClientLocked(source)
		if currentAgentID == "" || agentID != "" && currentAgentID != agentID {
			c.mu.Unlock()
			return nil, errors.New("ACP client changed before permission request")
		}
		agentID = currentAgentID
	}
	if session, ok := desktopSessionByID(c.state, params.SessionID); ok && agentID != "" && session.AgentID != agentID {
		c.mu.Unlock()
		return nil, errors.New("permission request agent does not own the session")
	}
	if c.permissionWait == nil {
		c.permissionWait = make(map[string]chan string)
	}
	if _, exists := c.permissionWait[item.RequestID]; exists {
		c.mu.Unlock()
		return nil, errors.New("duplicate permission request ID")
	}
	c.permissionWait[item.RequestID] = waiter
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:       desktopstate.EventPermissionRequested,
		SessionID:  params.SessionID,
		Permission: item,
	})
	statusAgentID := agentID
	if statusAgentID == "" {
		statusAgentID = c.activeAgentID
	}
	c.statuses[statusAgentID] = "Permission required · " + sessionTitle(c.state, params.SessionID)
	c.revision++
	c.mu.Unlock()
	c.notify()

	select {
	case <-ctx.Done():
		c.finishPermission(item.RequestID, params.SessionID, waiter)
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}, nil
	case optionID := <-waiter:
		c.finishPermission(item.RequestID, params.SessionID, waiter)
		return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}, nil
	}
}

func (c *controller) resolvePermission(requestID, optionID string) {
	c.mu.Lock()
	waiter := c.permissionWait[requestID]
	if waiter == nil || !permissionOptionAllowed(c.state, requestID, optionID) {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	select {
	case waiter <- optionID:
	default:
	}
}

func (c *controller) finishPermission(requestID, sessionID string, waiter chan string) {
	c.mu.Lock()
	if c.permissionWait[requestID] != waiter {
		c.mu.Unlock()
		return
	}
	delete(c.permissionWait, requestID)
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventPermissionResolved,
		SessionID: sessionID,
		RequestID: requestID,
	})
	agentID := ""
	if session, ok := desktopSessionByID(c.state, sessionID); ok {
		agentID = session.AgentID
	}
	if agentID == "" {
		agentID = c.activeAgentID
	}
	c.statuses[agentID] = "Connected · running"
	c.revision++
	c.mu.Unlock()
	c.notify()
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
	available := maxPermissionDetailBytes - len(permissionDetailPrefix)
	if len(text) > available {
		text = truncateUTF8Prefix(text, available-len(permissionTruncationMark))
		text = strings.TrimSpace(text) + permissionTruncationMark
	}
	return permissionDetailPrefix + text
}

func truncateUTF8Prefix(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func permissionOptionAllowed(state desktopstate.State, requestID, optionID string) bool {
	for _, request := range state.PermissionInbox {
		if request.RequestID != requestID {
			continue
		}
		for _, option := range request.Options {
			if option.ID == optionID {
				return true
			}
		}
	}
	return false
}

func sessionTitle(state desktopstate.State, sessionID string) string {
	session, ok := desktopSessionByID(state, sessionID)
	if !ok || strings.TrimSpace(session.Title) == "" {
		return "session"
	}
	return session.Title
}
