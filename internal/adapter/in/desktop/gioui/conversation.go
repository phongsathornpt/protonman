//go:build desktop || desktop_gio

package gioui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	subagentSessionUpdate       = "protonman_subagent_update"
	historyLoadTimeout          = 30 * time.Second
	maxHistoryStagingBytes      = 8 << 20
	maxHistoryStagingEvents     = 8192
	historyStagingEventOverhead = 128
	maxSessionTimelineBytes     = 8 << 20
	maxSessionTimelineItems     = 8192
	maxSessionSubagents         = 512
	timelineItemOverhead        = 64
	maxSubagentSummaryBytes     = 16 << 10
	messageStreamFlushInterval  = 16 * time.Millisecond
	maxMessageStreamBytes       = 4 << 20
	messageStreamTrimTarget     = 3 << 20
	stagingTrimTargetBytes      = maxHistoryStagingBytes * 3 / 4
	stagingTrimTargetEvents     = maxHistoryStagingEvents * 3 / 4
	timelineTrimTargetBytes     = maxSessionTimelineBytes * 3 / 4
	timelineTrimTargetItems     = maxSessionTimelineItems * 3 / 4
)

type historyState uint8

type sessionHistoryLoad struct {
	cancel context.CancelFunc
}

type messageStreamBuffer struct {
	sessionID string
	itemID    string
	kind      desktopstate.TimelineKind
	text      strings.Builder
	truncated bool
	timer     *time.Timer
}

type messageStreamKey struct {
	sessionID string
	kind      string
}

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
	session, sessionExists := desktopSessionByID(c.state, payload.SessionID)
	if !sessionExists || agentID != "" && session.AgentID != agentID {
		c.mu.Unlock()
		return
	}
	if payload.SessionID != c.state.ActiveSessionID {
		c.mu.Unlock()
		return
	}
	timelineEvent, ok := c.eventsForSessionUpdateLocked(payload, update)
	if !ok {
		c.mu.Unlock()
		return
	}
	if c.histories[payload.SessionID] == historyStateLoading {
		c.stageHistoryEventLocked(payload.SessionID, timelineEvent)
		c.mu.Unlock()
		return
	}
	if update.Kind == "user_message_chunk" || update.Kind == "agent_message_chunk" {
		c.appendMessageChunkLocked(payload.SessionID, update.Kind, timelineEvent)
		c.mu.Unlock()
		return
	}
	c.applyTimelineEventLocked(timelineEvent)
	c.revision++
	c.mu.Unlock()
	c.notify()
	if update.Kind == "tool_call_update" && terminalToolStatus(update.Status) {
		c.refreshSessionContextFrom(source, payload.SessionID, true)
		c.refreshSessionMemoryFrom(source, payload.SessionID, true)
	}
}

func (c *controller) stageHistoryEventLocked(sessionID string, event desktopstate.Event) {
	if c.historyStaging == nil {
		c.historyStaging = make(map[string][]desktopstate.Event)
	}
	if c.historyStagingBytes == nil {
		c.historyStagingBytes = make(map[string]int)
	}
	staged := c.historyStaging[sessionID]
	event, eventTruncated, keep := boundSessionEvent(event, maxHistoryStagingBytes)
	if eventTruncated {
		if c.historyStagingTruncated == nil {
			c.historyStagingTruncated = make(map[string]bool)
		}
		c.historyStagingTruncated[sessionID] = true
	}
	if !keep {
		return
	}
	size := historyEventSize(event)
	c.historyStaging[sessionID] = append(staged, event)
	c.historyStagingBytes[sessionID] += size
	if c.historyStagingBytes[sessionID] <= maxHistoryStagingBytes && len(c.historyStaging[sessionID]) <= maxHistoryStagingEvents {
		return
	}
	staged = c.historyStaging[sessionID]
	drop := 0
	bytes := c.historyStagingBytes[sessionID]
	for bytes > stagingTrimTargetBytes || len(staged)-drop > stagingTrimTargetEvents {
		bytes -= historyEventSize(staged[drop])
		drop++
	}
	remaining := append([]desktopstate.Event(nil), staged[drop:]...)
	c.historyStaging[sessionID] = remaining
	c.historyStagingBytes[sessionID] = bytes
	c.historyStagingTruncated[sessionID] = true
}

func historyEventSize(event desktopstate.Event) int {
	return historyStagingEventOverhead + len(event.SessionID) + len(event.Item.ID) + len(event.Item.Title) +
		len(event.Item.Status) + len(event.Item.Text) + len(event.Subagent.ID) + len(event.Subagent.Profile) +
		len(event.Subagent.Task) + len(event.Subagent.Summary) + len(event.Subagent.Status)
}

func boundSessionEvent(event desktopstate.Event, maxBytes int) (desktopstate.Event, bool, bool) {
	limit := maxBytes - timelineItemOverhead
	if event.Kind == desktopstate.EventSubagentUpserted {
		if len(event.Subagent.ID) > 256 {
			return desktopstate.Event{}, true, false
		}
		truncated := false
		for _, field := range []*string{&event.Subagent.ID, &event.Subagent.Profile, &event.Subagent.Task, &event.Subagent.Status, &event.Subagent.Summary} {
			maxFieldBytes := maxSubagentSummaryBytes
			switch field {
			case &event.Subagent.Profile, &event.Subagent.Status:
				maxFieldBytes = 128
			case &event.Subagent.Task:
				maxFieldBytes = 2048
			}
			if len(*field) > maxFieldBytes {
				*field = strings.Clone((*field)[:maxFieldBytes])
				truncated = true
			}
		}
		return event, truncated, event.Subagent.ID != ""
	}
	metadata := len(event.SessionID) + len(event.Item.ID) + len(event.Item.Title) + len(event.Item.Status) + timelineItemOverhead
	if metadata >= limit {
		return desktopstate.Event{}, true, false
	}
	maxText := limit - metadata
	if len(event.Item.Text) <= maxText {
		return event, false, true
	}
	text := event.Item.Text
	start := len(text) - maxText
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	event.Item.Text = strings.Clone(text[start:])
	return event, true, true
}

func timelineItemSize(item desktopstate.TimelineItem) int {
	return timelineItemOverhead + len(item.ID) + len(item.Title) + len(item.Status) + len(item.Text)
}

func (c *controller) appendMessageChunkLocked(sessionID, kind string, event desktopstate.Event) {
	streamKey := messageStreamKey{sessionID: sessionID, kind: kind}
	chunkText := event.Item.Text
	if c.messageStreamBuffers == nil {
		c.messageStreamBuffers = make(map[messageStreamKey]*messageStreamBuffer)
	}
	buffer := c.messageStreamBuffers[streamKey]
	if buffer == nil {
		buffer = &messageStreamBuffer{sessionID: sessionID, itemID: event.Item.ID, kind: event.Item.Kind}
		c.messageStreamBuffers[streamKey] = buffer
		found := false
		if session := desktopstateSessionPointer(&c.state, sessionID); session != nil {
			for _, item := range session.Timeline {
				if item.ID == event.Item.ID && item.Kind == event.Item.Kind {
					found = true
					break
				}
			}
		}
		if !found {
			event.Item.Text = ""
			c.applyTimelineEventLocked(event)
		}
	}
	buffer.append(chunkText)
	if buffer.timer == nil {
		buffer.timer = time.AfterFunc(messageStreamFlushInterval, func() {
			c.flushMessageStream(streamKey, buffer)
		})
	}
}

func (buffer *messageStreamBuffer) append(text string) {
	if text == "" {
		return
	}
	if buffer.text.Len()+len(text) <= maxMessageStreamBytes {
		buffer.text.WriteString(text)
		return
	}
	current := buffer.text.String()
	var retained string
	if len(text) >= messageStreamTrimTarget {
		retained = suffixBytes(text, messageStreamTrimTarget)
	} else {
		retained = suffixBytes(current+text, messageStreamTrimTarget)
	}
	buffer.text.Reset()
	buffer.text.WriteString(retained)
	buffer.truncated = true
}

func (c *controller) flushMessageStream(streamKey messageStreamKey, expected *messageStreamBuffer) {
	if c.ctx != nil && c.ctx.Err() != nil {
		return
	}
	c.mu.Lock()
	if c.messageStreamBuffers[streamKey] != expected {
		c.mu.Unlock()
		return
	}
	expected.timer = nil
	if c.flushMessageStreamLocked(expected) {
		c.revision++
		if !c.advanceSnapshotCacheForTimelineLocked(expected.sessionID) {
			c.snapshotCache = controllerSnapshotCache{}
		}
		c.mu.Unlock()
		c.notify()
		return
	}
	c.mu.Unlock()
}

func (c *controller) flushMessageStreamLocked(buffer *messageStreamBuffer) bool {
	session := desktopstateSessionPointer(&c.state, buffer.sessionID)
	if session == nil {
		return false
	}
	for index := range session.Timeline {
		item := &session.Timeline[index]
		if item.ID != buffer.itemID || item.Kind != buffer.kind {
			continue
		}
		changed := item.Text != buffer.text.String()
		if changed {
			if c.timelineBytes == nil {
				c.timelineBytes = make(map[string]int)
			}
			if _, ok := c.timelineBytes[buffer.sessionID]; !ok {
				c.timelineBytes[buffer.sessionID] = timelineSize(session.Timeline)
			}
			c.timelineBytes[buffer.sessionID] += len(buffer.text.String()) - len(item.Text)
			item.Text = buffer.text.String()
		}
		if buffer.truncated && !session.HistoryTruncated {
			session.HistoryTruncated = true
			changed = true
		}
		item.Streaming = true
		c.pruneSessionTimelineLocked(buffer.sessionID)
		return changed
	}
	return false
}

func (c *controller) flushMessageStreamsLocked(sessionID string) {
	for streamKey, buffer := range c.messageStreamBuffers {
		if streamKey.sessionID != sessionID {
			continue
		}
		if buffer.timer != nil {
			buffer.timer.Stop()
			buffer.timer = nil
		}
		c.flushMessageStreamLocked(buffer)
	}
}

func coalesceStagedMessageChunks(events []desktopstate.Event) []desktopstate.Event {
	type messageAccumulator struct {
		index int
		text  strings.Builder
	}
	accumulators := make(map[string]*messageAccumulator)
	coalesced := make([]desktopstate.Event, 0, len(events))
	for _, event := range events {
		item := event.Item
		if (event.Kind == desktopstate.EventTimelineAppended || event.Kind == desktopstate.EventTimelineUpserted) &&
			(item.Kind == desktopstate.TimelineUser || item.Kind == desktopstate.TimelineAssistant) && item.ID != "" {
			key := string(item.Kind) + "\x00" + item.ID
			accumulator := accumulators[key]
			if accumulator == nil {
				event.Item.Text = ""
				event.Item.Streaming = false
				accumulator = &messageAccumulator{index: len(coalesced)}
				accumulators[key] = accumulator
				coalesced = append(coalesced, event)
			}
			accumulator.text.WriteString(item.Text)
			continue
		}
		coalesced = append(coalesced, event)
	}
	for _, accumulator := range accumulators {
		coalesced[accumulator.index].Item.Text = accumulator.text.String()
	}
	return coalesced
}

func (c *controller) applyTimelineEventLocked(event desktopstate.Event) {
	session := desktopstateSessionPointer(&c.state, event.SessionID)
	if session == nil {
		return
	}
	bounded, truncated, keep := boundSessionEvent(event, maxSessionTimelineBytes)
	if truncated {
		session.HistoryTruncated = true
	}
	if !keep {
		return
	}
	event = bounded
	if event.Kind == desktopstate.EventSubagentUpserted {
		desktopstate.Apply(&c.state, event)
		if session := desktopstateSessionPointer(&c.state, event.SessionID); session != nil && len(session.Subagents) > maxSessionSubagents {
			drop := len(session.Subagents) - maxSessionSubagents*3/4
			for index := 0; index < drop; index++ {
				session.Subagents[index] = desktopstate.SubagentState{}
			}
			session.Subagents = append([]desktopstate.SubagentState(nil), session.Subagents[drop:]...)
			session.HistoryTruncated = true
		}
		return
	}
	if c.timelineBytes == nil {
		c.timelineBytes = make(map[string]int)
	}
	if _, ok := c.timelineBytes[event.SessionID]; !ok {
		c.timelineBytes[event.SessionID] = timelineSize(session.Timeline)
	}
	var delta int
	switch event.Kind {
	case desktopstate.EventTimelineAppended:
		delta = timelineItemSize(event.Item)
	case desktopstate.EventTimelineUpserted:
		previousSize := 0
		if event.Item.ID != "" {
			for _, previous := range session.Timeline {
				if previous.ID == event.Item.ID && previous.Kind == event.Item.Kind {
					previousSize = timelineItemSize(previous)
					delta = timelineItemSize(desktopstate.MergeTimelineItem(previous, event.Item)) - previousSize
					break
				}
			}
		}
		if previousSize == 0 {
			delta = timelineItemSize(event.Item)
		}
	default:
		return
	}
	desktopstate.Apply(&c.state, event)
	c.timelineBytes[event.SessionID] += delta
	c.pruneSessionTimelineLocked(event.SessionID)
}

func timelineSize(items []desktopstate.TimelineItem) int {
	bytes := 0
	for _, item := range items {
		bytes += timelineItemSize(item)
	}
	return bytes
}

func (c *controller) pruneSessionTimelineLocked(sessionID string) {
	session := desktopstateSessionPointer(&c.state, sessionID)
	if session == nil || c.timelineBytes[sessionID] <= maxSessionTimelineBytes && len(session.Timeline) <= maxSessionTimelineItems {
		return
	}
	bytes := 0
	for _, item := range session.Timeline {
		bytes += timelineItemSize(item)
	}
	if bytes <= maxSessionTimelineBytes && len(session.Timeline) <= maxSessionTimelineItems {
		c.timelineBytes[sessionID] = bytes
		return
	}
	drop := 0
	for drop < len(session.Timeline) && (bytes > timelineTrimTargetBytes || len(session.Timeline)-drop > timelineTrimTargetItems) {
		if len(session.Timeline)-drop == 1 && bytes > timelineTrimTargetBytes {
			item := &session.Timeline[drop]
			metadata := timelineItemSize(*item) - len(item.Text)
			maxText := timelineTrimTargetBytes - metadata
			if maxText > 0 && len(item.Text) > maxText {
				item.Text = suffixBytes(item.Text, maxText)
				bytes = timelineItemSize(*item)
				break
			}
		}
		bytes -= timelineItemSize(session.Timeline[drop])
		session.Timeline[drop] = desktopstate.TimelineItem{}
		drop++
	}
	remaining := append([]desktopstate.TimelineItem(nil), session.Timeline[drop:]...)
	session.Timeline = remaining
	session.HistoryTruncated = true
	c.timelineBytes[sessionID] = bytes
}

func desktopstateSessionPointer(state *desktopstate.State, sessionID string) *desktopstate.SessionState {
	for index := range state.Sessions {
		if state.Sessions[index].ID == sessionID {
			return &state.Sessions[index]
		}
	}
	return nil
}

func suffixBytes(text string, maxBytes int) string {
	if len(text) <= maxBytes {
		return text
	}
	start := len(text) - maxBytes
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	return strings.Clone(text[start:])
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
		streamKey := messageStreamKey{sessionID: update.SessionID, kind: update.Kind}
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
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	type contentBlock struct {
		Text    string `json:"text"`
		Content *struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	switch raw[0] {
	case '{':
		var block contentBlock
		if json.Unmarshal(raw, &block) != nil {
			return ""
		}
		if block.Text != "" {
			return block.Text
		}
		if block.Content != nil {
			return block.Content.Text
		}
		return ""
	case '[':
		var blocks []contentBlock
		if json.Unmarshal(raw, &blocks) != nil {
			return ""
		}
		textBytes := 0
		for _, item := range blocks {
			if item.Text != "" {
				textBytes += len(item.Text)
			} else if item.Content != nil {
				textBytes += len(item.Content.Text)
			}
		}
		var out strings.Builder
		out.Grow(textBytes)
		for _, item := range blocks {
			if item.Text != "" {
				out.WriteString(item.Text)
			} else if item.Content != nil {
				out.WriteString(item.Content.Text)
			}
		}
		return out.String()
	default:
		return ""
	}
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
	base := c.ctx
	if base == nil {
		base = context.Background()
	}
	callCtx, cancel := context.WithTimeout(base, historyLoadTimeout)
	request := &sessionHistoryLoad{cancel: cancel}
	if c.historyLoads == nil {
		c.historyLoads = make(map[string]*sessionHistoryLoad)
	}
	c.historyLoads[sessionID] = request
	c.histories[sessionID] = historyStateLoading
	c.historyStaging[sessionID] = nil
	session.HistoryTruncated = false
	if c.timelineBytes == nil {
		c.timelineBytes = make(map[string]int)
	}
	c.timelineBytes[sessionID] = timelineSize(session.Timeline)
	if c.historyStagingBytes == nil {
		c.historyStagingBytes = make(map[string]int)
	}
	c.historyStagingBytes[sessionID] = 0
	delete(c.historyStagingTruncated, sessionID)
	c.clearMessageStreamsLocked(sessionID)
	c.revision++
	c.mu.Unlock()
	c.notify()
	params := c.mcpSessionParams(sessionID, workspace, session.AdditionalDirectories)

	go func() {
		unlock, acquired := c.lockAgentSessionContext(callCtx, agentID)
		if !acquired {
			c.finishSessionHistoryLoadRequest(client, sessionID, request, callCtx.Err())
			return
		}
		defer unlock()
		c.mu.Lock()
		currentRequest := c.historyLoads[sessionID] == request
		activeSession := c.state.ActiveSessionID == sessionID
		c.mu.Unlock()
		if !currentRequest {
			return
		}
		if !activeSession {
			c.finishSessionHistoryLoadRequest(client, sessionID, request, context.Canceled)
			return
		}
		err := client.Call(callCtx, "session/load", params, nil)
		if isACPMethodNotFound(err) {
			err = nil
		}
		c.finishSessionHistoryLoadRequest(client, sessionID, request, err)
	}()
}

func (c *controller) finishSessionHistoryLoad(client *acpclient.Client, sessionID string, loadErr error) {
	c.finishSessionHistoryLoadRequest(client, sessionID, nil, loadErr)
}

func (c *controller) finishSessionHistoryLoadRequest(client *acpclient.Client, sessionID string, request *sessionHistoryLoad, loadErr error) {
	c.mu.Lock()
	if request != nil {
		if c.historyLoads[sessionID] != request {
			c.mu.Unlock()
			return
		}
		delete(c.historyLoads, sessionID)
		request.cancel()
	}
	current, agentID := c.clientForSessionLocked(sessionID)
	currentClient := current != nil && current == client
	if !currentClient {
		loadErr = errSessionHistoryClientChanged
	}
	staged := c.historyStaging[sessionID]
	truncated := c.historyStagingTruncated[sessionID]
	delete(c.historyStaging, sessionID)
	delete(c.historyStagingBytes, sessionID)
	delete(c.historyStagingTruncated, sessionID)
	c.clearMessageStreamsLocked(sessionID)
	if loadErr == nil && c.state.ActiveSessionID == sessionID {
		if session := desktopstateSessionPointer(&c.state, sessionID); session != nil {
			session.HistoryTruncated = session.HistoryTruncated || truncated
		}
		for _, event := range coalesceStagedMessageChunks(staged) {
			c.applyTimelineEventLocked(event)
		}
		c.clearMessageStreamsLocked(sessionID)
		c.pruneSessionTimelineLocked(sessionID)
		c.histories[sessionID] = historyStateLoaded
		if truncated {
			c.statuses[agentID] = "Connected · recent session history loaded"
		} else {
			c.statuses[agentID] = "Connected · session history loaded"
		}
	} else if loadErr == nil {
		c.histories[sessionID] = historyStateUnloaded
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
	c.applyTimelineEventLocked(desktopstate.Event{
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
		c.applyTimelineEventLocked(desktopstate.Event{
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
			delete(c.timelineBytes, sessionID)
		}
	}
	for sessionID, load := range c.historyLoads {
		if _, ok := live[sessionID]; !ok {
			load.cancel()
			delete(c.historyLoads, sessionID)
		}
	}
	for sessionID := range c.historyStaging {
		if _, ok := live[sessionID]; !ok {
			delete(c.historyStaging, sessionID)
			delete(c.historyStagingBytes, sessionID)
			delete(c.historyStagingTruncated, sessionID)
		}
	}
	for sessionID := range c.historyStagingTruncated {
		if _, ok := live[sessionID]; !ok {
			delete(c.historyStagingTruncated, sessionID)
		}
	}
	for sessionID := range c.messageSequence {
		if _, ok := live[sessionID]; !ok {
			delete(c.messageSequence, sessionID)
		}
	}
	for streamKey := range c.messageStreams {
		if _, ok := live[streamKey.sessionID]; !ok {
			if buffer := c.messageStreamBuffers[streamKey]; buffer != nil && buffer.timer != nil {
				buffer.timer.Stop()
			}
			delete(c.messageStreamBuffers, streamKey)
			delete(c.messageStreams, streamKey)
		}
	}
	permissions := c.state.PermissionInbox[:0]
	livePermissionRequests := make(map[string]struct{}, len(c.state.PermissionInbox))
	for _, permission := range c.state.PermissionInbox {
		if _, ok := live[permission.SessionID]; ok {
			permissions = append(permissions, permission)
			livePermissionRequests[permission.RequestID] = struct{}{}
		}
	}
	c.state.PermissionInbox = permissions
	for requestID := range c.permissionWait {
		if _, ok := livePermissionRequests[requestID]; !ok {
			select {
			case c.permissionWait[requestID] <- "":
			default:
			}
			delete(c.permissionWait, requestID)
		}
	}
	if _, ok := live[c.runtimeMutation]; !ok {
		c.runtimeMutation = ""
	}
	c.pruneAgentRuntimeLocked()
}

// pruneInactiveSessionHistoryLocked keeps only the selected session's transcript
// resident. ACP session/load reconstructs an evicted transcript when selected.
func (c *controller) pruneInactiveSessionHistoryLocked(activeSessionID string) {
	for index := range c.state.Sessions {
		session := &c.state.Sessions[index]
		if session.ID == activeSessionID {
			continue
		}
		session.Timeline = nil
		session.HistoryTruncated = false
		session.Subagents = nil
		session.Context = desktopstate.SessionContextState{}
		session.Runtime = desktopstate.RuntimeSettingsState{}
		delete(c.timelineBytes, session.ID)
		if load := c.historyLoads[session.ID]; load != nil {
			load.cancel()
			delete(c.historyLoads, session.ID)
			c.histories[session.ID] = historyStateUnloaded
		}
		delete(c.historyStaging, session.ID)
		delete(c.historyStagingBytes, session.ID)
		delete(c.historyStagingTruncated, session.ID)
		if c.histories[session.ID] != historyStateLoading {
			c.histories[session.ID] = historyStateUnloaded
		}
		for streamKey := range c.messageStreams {
			if streamKey.sessionID == session.ID {
				if buffer := c.messageStreamBuffers[streamKey]; buffer != nil && buffer.timer != nil {
					buffer.timer.Stop()
				}
				delete(c.messageStreamBuffers, streamKey)
				delete(c.messageStreams, streamKey)
			}
		}
	}
}

func (c *controller) resetTransientSessionStateLocked() {
	for sessionID, state := range c.histories {
		if state == historyStateLoading {
			c.histories[sessionID] = historyStateUnloaded
		}
	}
	for sessionID, load := range c.historyLoads {
		load.cancel()
		delete(c.historyLoads, sessionID)
	}
	clear(c.historyStaging)
	clear(c.historyStagingBytes)
	clear(c.historyStagingTruncated)
	clear(c.messageStreams)
	for _, buffer := range c.messageStreamBuffers {
		if buffer.timer != nil {
			buffer.timer.Stop()
		}
	}
	clear(c.messageStreamBuffers)
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
		if load := c.historyLoads[session.ID]; load != nil {
			load.cancel()
			delete(c.historyLoads, session.ID)
		}
		if c.histories[session.ID] == historyStateLoading {
			c.histories[session.ID] = historyStateUnloaded
		}
		delete(c.historyStaging, session.ID)
		delete(c.historyStagingBytes, session.ID)
		delete(c.historyStagingTruncated, session.ID)
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
	var session *desktopstate.SessionState
	if index, found := sessionIndex(c.state.Sessions, sessionID); found {
		session = &c.state.Sessions[index]
	}
	for streamKey := range c.messageStreams {
		if streamKey.sessionID == sessionID {
			if buffer := c.messageStreamBuffers[streamKey]; buffer != nil {
				if buffer.timer != nil {
					buffer.timer.Stop()
					buffer.timer = nil
				}
				c.flushMessageStreamLocked(buffer)
				delete(c.messageStreamBuffers, streamKey)
			}
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
