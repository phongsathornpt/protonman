//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	subagentSessionUpdate = "protonman_subagent_update"
	historyLoadTimeout    = 30 * time.Second
)

type historyState uint8

const (
	historyStateUnloaded historyState = iota
	historyStateLoading
	historyStateLoaded
)

var errSessionHistoryClientChanged = errors.New("ACP client changed during history load")

type sessionUpdatePayload struct {
	SessionID string `json:"sessionId"`
	Update    struct {
		Kind       string          `json:"sessionUpdate"`
		ToolCallID string          `json:"toolCallId"`
		Title      string          `json:"title"`
		Status     string          `json:"status"`
		Content    json.RawMessage `json:"content"`
		AgentID    string          `json:"agentId"`
		Profile    string          `json:"profile"`
		Task       string          `json:"task"`
		Summary    string          `json:"summary"`
	} `json:"update"`
}

func (c *controller) handleACPEvent(event acpclient.Event) {
	c.handleACPEventFrom(nil, event)
}

func (c *controller) handleACPEventFor(client *acpclient.Client, event acpclient.Event) {
	c.handleACPEventFrom(client, event)
}

func (c *controller) handleACPEventForAgent(agentID string, client *acpclient.Client, event acpclient.Event) {
	c.handleACPEventFromAgent(agentID, client, event)
}

func (c *controller) handleACPEventFrom(source *acpclient.Client, event acpclient.Event) {
	c.handleACPEventFromAgent("", source, event)
}

func (c *controller) handleACPEventFromAgent(agentID string, source *acpclient.Client, event acpclient.Event) {
	if event.Method != "session/update" {
		return
	}
	var payload sessionUpdatePayload
	if json.Unmarshal(event.Params, &payload) != nil || strings.TrimSpace(payload.SessionID) == "" {
		return
	}
	update := desktopstate.SessionUpdate{
		SessionID:  payload.SessionID,
		Kind:       payload.Update.Kind,
		ToolCallID: payload.Update.ToolCallID,
		Title:      payload.Update.Title,
		Status:     payload.Update.Status,
		Text:       sessionUpdateText(payload.Update.Content),
	}

	c.mu.Lock()
	if source != nil {
		currentAgentID := c.agentIDForClientLocked(source)
		if currentAgentID == "" || agentID != "" && currentAgentID != agentID {
			c.mu.Unlock()
			return
		}
		agentID = currentAgentID
	}
	if session, ok := desktopSessionByID(c.state, payload.SessionID); ok && agentID != "" && session.AgentID != agentID {
		c.mu.Unlock()
		return
	}
	timelineEvent, ok := c.eventsForSessionUpdateLocked(payload, update)
	if !ok {
		c.mu.Unlock()
		return
	}
	if c.histories[payload.SessionID] == historyStateLoading {
		c.historyStaging[payload.SessionID] = append(c.historyStaging[payload.SessionID], timelineEvent)
		c.mu.Unlock()
		return
	}
	desktopstate.Apply(&c.state, timelineEvent)
	c.revision++
	c.mu.Unlock()
	c.notify()
	if update.Kind == "tool_call_update" && terminalToolStatus(update.Status) {
		c.refreshSessionContextFrom(source, payload.SessionID, true)
		c.refreshSessionMemoryFrom(source, payload.SessionID, true)
	}
}

func (c *controller) eventsForSessionUpdateLocked(payload sessionUpdatePayload, update desktopstate.SessionUpdate) (desktopstate.Event, bool) {
	if update.SessionID == "" {
		return desktopstate.Event{}, false
	}
	if update.Kind == subagentSessionUpdate {
		agentID := strings.TrimSpace(payload.Update.AgentID)
		if agentID == "" {
			return desktopstate.Event{}, false
		}
		return desktopstate.Event{
			Kind:      desktopstate.EventSubagentUpserted,
			SessionID: update.SessionID,
			Subagent: desktopstate.SubagentState{
				ID:      agentID,
				Profile: strings.ToLower(strings.TrimSpace(payload.Update.Profile)),
				Task:    strings.TrimSpace(payload.Update.Task),
				Summary: strings.TrimSpace(payload.Update.Summary),
				Status:  strings.TrimSpace(payload.Update.Status),
			},
		}, true
	}

	switch update.Kind {
	case "user_message_chunk", "agent_message_chunk":
		if update.Text == "" {
			return desktopstate.Event{}, false
		}
		streamKey := update.SessionID + "\x00" + update.Kind
		itemID := c.messageStreams[streamKey]
		eventKind := desktopstate.EventTimelineUpserted
		if itemID == "" {
			eventKind = desktopstate.EventTimelineAppended
			itemID = c.nextTimelineIDLocked(update.SessionID, update.Kind)
			c.messageStreams[streamKey] = itemID
		}
		itemKind := desktopstate.TimelineUser
		if update.Kind == "agent_message_chunk" {
			itemKind = desktopstate.TimelineAssistant
		}
		return desktopstate.Event{
			Kind:      eventKind,
			SessionID: update.SessionID,
			Item: desktopstate.TimelineItem{
				Kind:      itemKind,
				ID:        itemID,
				Text:      update.Text,
				Streaming: true,
			},
		}, true
	case "tool_call", "tool_call_update":
		c.clearMessageStreamsLocked(update.SessionID)
		event, ok := desktopstate.TimelineEvent(update)
		if !ok {
			return desktopstate.Event{}, false
		}
		return event, true
	default:
		return desktopstate.Event{}, false
	}
}

func sessionUpdateText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var block struct {
		Text    string `json:"text"`
		Content *struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &block) == nil && block.Text != "" {
		return block.Text
	}
	if block.Content != nil && block.Content.Text != "" {
		return block.Content.Text
	}
	var blocks []struct {
		Text    string `json:"text"`
		Content *struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var out strings.Builder
	for _, item := range blocks {
		if item.Text != "" {
			out.WriteString(item.Text)
			continue
		}
		if item.Content != nil {
			out.WriteString(item.Content.Text)
		}
	}
	return out.String()
}

func (c *controller) loadSessionHistory(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	if state := c.histories[sessionID]; state == historyStateLoading || state == historyStateLoaded {
		c.mu.Unlock()
		return
	}
	session, ok := desktopSessionByID(c.state, sessionID)
	workspace := strings.TrimSpace(session.Workspace)
	if !ok || workspace == "" {
		c.mu.Unlock()
		return
	}
	client, agentID := c.clientForSessionLocked(sessionID)
	if client == nil || c.connections[agentID] != connectionConnected {
		c.mu.Unlock()
		return
	}
	c.histories[sessionID] = historyStateLoading
	c.historyStaging[sessionID] = nil
	c.clearMessageStreamsLocked(sessionID)
	c.revision++
	c.mu.Unlock()
	c.notify()
	params := c.mcpSessionParams(sessionID, workspace, session.AdditionalDirectories)

	go func() {
		defer c.lockAgentSession(agentID)()
		callCtx, cancel := context.WithTimeout(c.ctx, historyLoadTimeout)
		defer cancel()
		err := client.Call(callCtx, "session/load", params, nil)
		if isACPMethodNotFound(err) {
			err = nil
		}
		c.finishSessionHistoryLoad(client, sessionID, err)
	}()
}

func (c *controller) finishSessionHistoryLoad(client *acpclient.Client, sessionID string, loadErr error) {
	c.mu.Lock()
	current, agentID := c.clientForSessionLocked(sessionID)
	currentClient := current != nil && current == client
	if !currentClient {
		loadErr = errSessionHistoryClientChanged
	}
	staged := c.historyStaging[sessionID]
	delete(c.historyStaging, sessionID)
	c.clearMessageStreamsLocked(sessionID)
	if loadErr == nil {
		for _, event := range staged {
			desktopstate.Apply(&c.state, event)
		}
		c.clearMessageStreamsLocked(sessionID)
		c.histories[sessionID] = historyStateLoaded
		c.statuses[agentID] = "Connected · session history loaded"
	} else {
		c.histories[sessionID] = historyStateUnloaded
		c.statuses[agentID] = "Session history failed · " + compactError(loadErr)
	}
	activeSessionID := c.state.ActiveSessionID
	c.revision++
	c.mu.Unlock()
	c.notify()
	if currentClient && loadErr == nil && activeSessionID == sessionID {
		c.refreshSessionContext(sessionID, false)
		c.refreshSessionMemory(sessionID, false)
		c.refreshSessionRuntime(sessionID, false)
	}
}

func (c *controller) resumeKnownSessions(agentID string, client *acpclient.Client) {
	c.mu.RLock()
	sessions := append([]desktopstate.SessionState(nil), c.state.Sessions...)
	c.mu.RUnlock()
	defer c.lockAgentSession(agentID)()
	for _, session := range sessions {
		if c.ctx.Err() != nil {
			return
		}
		workspace := strings.TrimSpace(session.Workspace)
		if session.ID == "" || workspace == "" || session.AgentID != agentID && !(session.AgentID == "" && agentID == controllerAgentID) {
			continue
		}
		params := c.mcpSessionParams(session.ID, workspace, session.AdditionalDirectories)
		callCtx, cancel := context.WithTimeout(c.ctx, reconnectRequestTimeout)
		err := client.Call(callCtx, "session/resume", params, nil)
		cancel()
		if err != nil && !isACPMethodNotFound(err) && c.ctx.Err() == nil {
			c.setAgentStatus(agentID, "Session resume failed · "+compactError(err))
		}
	}
}

func (c *controller) sendPrompt(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	mcpServers := c.mcpServersPayload()
	c.mu.Lock()
	sessionID := c.state.ActiveSessionID
	session, ok := desktopSessionByID(c.state, sessionID)
	client, agentID := c.clientForSessionLocked(sessionID)
	if !ok || client == nil || c.connections[agentID] != connectionConnected || sessionBusy(session.Status) || c.histories[sessionID] == historyStateLoading || strings.TrimSpace(session.Workspace) == "" {
		c.mu.Unlock()
		return
	}
	c.clearMessageStreamsLocked(sessionID)
	desktopstate.Apply(&c.state, desktopstate.Event{Kind: desktopstate.EventPromptStarted, SessionID: sessionID})
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventTimelineAppended,
		SessionID: sessionID,
		Item: desktopstate.TimelineItem{
			Kind: desktopstate.TimelineUser,
			ID:   c.nextTimelineIDLocked(sessionID, "prompt"),
			Text: text,
		},
	})
	c.statuses[agentID] = "Running · " + session.Title
	c.revision++
	c.mu.Unlock()
	c.notify()

	go c.runPrompt(client, session, text, mcpServers)
}

func (c *controller) runPrompt(client *acpclient.Client, session desktopstate.SessionState, text string, mcpServers []map[string]any) {
	defer c.lockAgentSession(session.AgentID)()
	params := map[string]any{"sessionId": session.ID, "cwd": session.Workspace}
	if len(session.AdditionalDirectories) > 0 {
		params["additionalDirectories"] = session.AdditionalDirectories
	}
	if len(mcpServers) > 0 {
		params["mcpServers"] = mcpServers
	}
	err := client.Call(c.ctx, "session/resume", params, nil)
	if err != nil && isACPMethodNotFound(err) {
		err = nil
	}
	var result struct {
		StopReason string `json:"stopReason"`
	}
	if err == nil {
		err = client.Call(c.ctx, "session/prompt", map[string]any{
			"sessionId": session.ID,
			"prompt":    []map[string]any{{"type": "text", "text": text}},
		}, &result)
	}

	c.mu.Lock()
	if !c.clientCurrentLocked(session.AgentID, client) {
		c.mu.Unlock()
		return
	}
	c.clearMessageStreamsLocked(session.ID)
	if err != nil {
		desktopstate.Apply(&c.state, desktopstate.Event{Kind: desktopstate.EventPromptFailed, SessionID: session.ID})
		desktopstate.Apply(&c.state, desktopstate.Event{
			Kind:      desktopstate.EventTimelineAppended,
			SessionID: session.ID,
			Item: desktopstate.TimelineItem{
				Kind: desktopstate.TimelineStatus,
				ID:   c.nextTimelineIDLocked(session.ID, "error"),
				Text: "Prompt failed: " + compactError(err),
			},
		})
		c.statuses[session.AgentID] = "Prompt failed · " + compactError(err)
	} else {
		desktopstate.Apply(&c.state, desktopstate.Event{Kind: desktopstate.EventPromptCompleted, SessionID: session.ID})
		stopReason := strings.TrimSpace(result.StopReason)
		if stopReason == "" {
			stopReason = "completed"
		}
		c.statuses[session.AgentID] = "Connected · " + stopReason
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
	c.refreshSessionContext(session.ID, false)
	c.refreshSessionMemory(session.ID, false)
	c.refreshSessionRuntime(session.ID, false)
}

func (c *controller) cancelPrompt() {
	c.mu.RLock()
	sessionID := c.state.ActiveSessionID
	session, ok := desktopSessionByID(c.state, sessionID)
	client, agentID := c.clientForSessionLocked(sessionID)
	c.mu.RUnlock()
	if !ok || client == nil || !sessionBusy(session.Status) {
		return
	}
	c.setAgentStatus(agentID, "Cancelling…")
	go func() {
		callCtx, cancel := context.WithTimeout(c.ctx, reconnectRequestTimeout)
		defer cancel()
		if err := client.Call(callCtx, "session/cancel", map[string]any{"sessionId": sessionID}, nil); err != nil && c.ctx.Err() == nil {
			c.mu.Lock()
			if c.clients[agentID] == client {
				c.statuses[agentID] = "Cancel failed · " + compactError(err)
				c.revision++
			}
			c.mu.Unlock()
			c.notify()
		}
	}()
}

func (c *controller) pruneSessionRuntimeLocked() {
	live := make(map[string]struct{}, len(c.state.Sessions))
	for _, session := range c.state.Sessions {
		live[session.ID] = struct{}{}
	}
	for sessionID := range c.histories {
		if _, ok := live[sessionID]; !ok {
			delete(c.histories, sessionID)
			delete(c.historyStaging, sessionID)
			delete(c.messageSequence, sessionID)
			c.clearMessageStreamsLocked(sessionID)
		}
	}
	permissions := c.state.PermissionInbox[:0]
	for _, permission := range c.state.PermissionInbox {
		if _, ok := live[permission.SessionID]; ok {
			permissions = append(permissions, permission)
		}
	}
	c.state.PermissionInbox = permissions
	if _, ok := live[c.runtimeMutation]; !ok {
		c.runtimeMutation = ""
	}
}

func (c *controller) resetTransientSessionStateLocked() {
	for sessionID, state := range c.histories {
		if state == historyStateLoading {
			c.histories[sessionID] = historyStateUnloaded
		}
	}
	clear(c.historyStaging)
	clear(c.messageStreams)
	c.permissionWait = make(map[string]chan string)
	c.runtimeMutation = ""
	c.contextRefresh.reset()
	c.memoryRefresh.reset()
	c.runtimeRefresh.reset()
}

func (c *controller) resetAgentTransientSessionStateLocked(agentID string) {
	for _, session := range c.state.Sessions {
		if session.AgentID != agentID {
			continue
		}
		if c.histories[session.ID] == historyStateLoading {
			c.histories[session.ID] = historyStateUnloaded
		}
		delete(c.historyStaging, session.ID)
		c.clearMessageStreamsLocked(session.ID)
		for _, permission := range c.state.PermissionInbox {
			if permission.SessionID != session.ID {
				continue
			}
			delete(c.permissionWait, permission.RequestID)
		}
		if c.runtimeMutation == session.ID {
			c.runtimeMutation = ""
		}
	}
}

func (c *controller) nextTimelineIDLocked(sessionID, prefix string) string {
	c.messageSequence[sessionID]++
	return fmt.Sprintf("%s-%d", prefix, c.messageSequence[sessionID])
}

func (c *controller) clearMessageStreamsLocked(sessionID string) {
	prefix := sessionID + "\x00"
	var session *desktopstate.SessionState
	if index, found := sessionIndex(c.state.Sessions, sessionID); found {
		session = &c.state.Sessions[index]
	}
	for streamKey := range c.messageStreams {
		if strings.HasPrefix(streamKey, prefix) {
			if session != nil {
				for index := range session.Timeline {
					if session.Timeline[index].ID == c.messageStreams[streamKey] {
						session.Timeline[index].Streaming = false
						break
					}
				}
			}
			delete(c.messageStreams, streamKey)
		}
	}
}

func sessionIndex(sessions []desktopstate.SessionState, sessionID string) (int, bool) {
	for index := range sessions {
		if sessions[index].ID == sessionID {
			return index, true
		}
	}
	return 0, false
}

func desktopSessionByID(state desktopstate.State, sessionID string) (desktopstate.SessionState, bool) {
	for _, session := range state.Sessions {
		if session.ID == sessionID {
			return session, true
		}
	}
	return desktopstate.SessionState{}, false
}

func sessionBusy(status desktopstate.TaskStatus) bool {
	switch status {
	case desktopstate.TaskQueued, desktopstate.TaskRunning, desktopstate.TaskWaitingPermission, desktopstate.TaskWaitingUser:
		return true
	default:
		return false
	}
}
