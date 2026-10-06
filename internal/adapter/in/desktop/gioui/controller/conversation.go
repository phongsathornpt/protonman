//go:build desktop || desktop_gio

package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
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

type HistoryState uint8

type sessionHistoryLoad struct {
	agentID    string
	sessionID  string
	storageKey string
	cancel     context.CancelFunc
	startedAt  time.Time
	// serverMeta carries the `_meta` returned by session/load so diagnostic
	// timings emitted by the agent can be reported alongside the client timing.
	serverMeta json.RawMessage
}

type messageStreamBuffer struct {
	agentID   string
	sessionID string
	itemID    string
	kind      desktopstate.TimelineKind
	// index caches the buffer item's position in the session timeline so a
	// stream flush can update it without rescanning the whole timeline. It is
	// repaired whenever retention pruning shifts the timeline.
	index     int
	text      strings.Builder
	truncated bool
	timer     *time.Timer
}

type messageStreamKey struct {
	agentID   string
	sessionID string
	kind      string
}

const (
	HistoryStateUnloaded HistoryState = iota
	HistoryStateLoading
	HistoryStateLoaded
)

var errSessionHistoryClientChanged = errors.New("ACP client changed during history load")

type sessionUpdatePayload struct {
	SessionID string `json:"sessionId"`
	Update    struct {
		Kind          string            `json:"sessionUpdate"`
		ToolCallID    string            `json:"toolCallId"`
		Title         string            `json:"title"`
		Status        string            `json:"status"`
		Content       json.RawMessage   `json:"content"`
		AgentID       string            `json:"agentId"`
		Profile       string            `json:"profile"`
		Task          string            `json:"task"`
		Summary       string            `json:"summary"`
		ModeID        string            `json:"modeId"`
		ConfigOptions []acpConfigOption `json:"configOptions"`
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
		AgentID:    agentID,
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
	update.AgentID = agentID
	session, sessionExists := SessionByID(c.state, payload.SessionID, agentID)
	if !sessionExists || agentID != "" && session.AgentID != agentID {
		c.mu.Unlock()
		return
	}
	if update.Kind == "current_mode_update" {
		modeID := strings.TrimSpace(payload.Update.ModeID)
		if modeID != "" {
			if sess := desktopstateSessionPointer(&c.state, payload.SessionID, session.AgentID); sess != nil {
				if isClineACPProfile(c.profiles[session.AgentID]) {
					autoApprove := sess.Runtime.PermissionMode == "always-approve"
					sess.Runtime.PermissionMode = clinePermissionMode(modeID, autoApprove)
				} else {
					sess.Runtime.PermissionMode = modeID
				}
				c.revision++
				c.mu.Unlock()
				c.notify()
				return
			}
		}
		c.mu.Unlock()
		return
	}
	if update.Kind == "config_option_update" {
		if sess := desktopstateSessionPointer(&c.state, payload.SessionID, session.AgentID); sess != nil {
			if isClineACPProfile(c.profiles[session.AgentID]) {
				modeID := clineModeID(payload.Update.ConfigOptions, sess.Runtime.PermissionMode)
				autoApprove := clineAutoApproveValue(payload.Update.ConfigOptions, sess.Runtime.PermissionMode == "always-approve")
				sess.Runtime.PermissionMode = clinePermissionMode(modeID, autoApprove)
			} else {
				for _, opt := range payload.Update.ConfigOptions {
					val := strings.TrimSpace(acpConfigStringValue(opt.CurrentValue))
					if val == "" {
						continue
					}
					switch opt.ID {
					case "permissionMode":
						sess.Runtime.PermissionMode = val
					case "model":
						sess.Runtime.Model = val
					case "reasoningEffort":
						sess.Runtime.Reasoning = val
					}
				}
			}
			c.revision++
		}
		c.mu.Unlock()
		c.notify()
		return
	}
	if payload.SessionID != c.state.ActiveSessionID || c.state.ActiveAgentID != "" && agentID != "" && c.state.ActiveAgentID != agentID {
		c.mu.Unlock()
		return
	}
	timelineEvent, ok := c.eventsForSessionUpdateLocked(payload, update)
	if !ok {
		c.mu.Unlock()
		return
	}
	sessionKey := desktopSessionStorageKey(c.state, session.Ref())
	if c.histories[sessionKey] == HistoryStateLoading {
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
		c.refreshSessionContextFrom(source, payload.SessionID, false, agentID)
		c.refreshSessionMemoryFrom(source, payload.SessionID, false, agentID)
	}
}

func (c *controller) stageHistoryEventLocked(sessionID string, event desktopstate.Event) {
	sessionKey := desktopSessionStorageKeyForID(c.state, sessionID, event.AgentID)
	if c.historyStaging == nil {
		c.historyStaging = make(map[string][]desktopstate.Event)
	}
	if c.historyStagingBytes == nil {
		c.historyStagingBytes = make(map[string]int)
	}
	staged := c.historyStaging[sessionKey]
	event, eventTruncated, keep := boundSessionEvent(event, maxHistoryStagingBytes)
	if eventTruncated {
		if c.historyStagingTruncated == nil {
			c.historyStagingTruncated = make(map[string]bool)
		}
		c.historyStagingTruncated[sessionKey] = true
	}
	if !keep {
		return
	}
	size := historyEventSize(event)
	c.historyStaging[sessionKey] = append(staged, event)
	c.historyStagingBytes[sessionKey] += size
	if c.historyStagingBytes[sessionKey] <= maxHistoryStagingBytes && len(c.historyStaging[sessionKey]) <= maxHistoryStagingEvents {
		return
	}
	staged = c.historyStaging[sessionKey]
	drop := 0
	bytes := c.historyStagingBytes[sessionKey]
	for bytes > stagingTrimTargetBytes || len(staged)-drop > stagingTrimTargetEvents {
		bytes -= historyEventSize(staged[drop])
		staged[drop] = desktopstate.Event{}
		drop++
	}
	if drop > 0 {
		copy(staged, staged[drop:])
		for i := len(staged) - drop; i < len(staged); i++ {
			staged[i] = desktopstate.Event{}
		}
		c.historyStaging[sessionKey] = staged[:len(staged)-drop]
	}
	c.historyStagingBytes[sessionKey] = bytes
	c.historyStagingTruncated[sessionKey] = true
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
	streamKey := messageStreamKey{agentID: event.AgentID, sessionID: sessionID, kind: kind}
	chunkText := event.Item.Text
	if c.messageStreamBuffers == nil {
		c.messageStreamBuffers = make(map[messageStreamKey]*messageStreamBuffer)
	}
	buffer := c.messageStreamBuffers[streamKey]
	if buffer == nil {
		buffer = &messageStreamBuffer{agentID: event.AgentID, sessionID: sessionID, itemID: event.Item.ID, kind: event.Item.Kind, index: -1}
		c.messageStreamBuffers[streamKey] = buffer
		found := false
		if session := desktopstateSessionPointer(&c.state, sessionID, event.AgentID); session != nil {
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
		if !c.advanceSnapshotCacheForTimelineLocked(expected.sessionID, expected.agentID) {
			c.snapshotCache = SnapshotCache{}
		}
		c.mu.Unlock()
		c.notify()
		return
	}
	c.mu.Unlock()
}

func (c *controller) flushMessageStreamLocked(buffer *messageStreamBuffer) bool {
	session := desktopstateSessionPointer(&c.state, buffer.sessionID, buffer.agentID)
	if session == nil {
		return false
	}
	sessionKey := desktopSessionStorageKey(c.state, session.Ref())
	index := timelineItemIndex(session, buffer)
	if index < 0 {
		return false
	}
	item := &session.Timeline[index]
	next := buffer.text.String()
	changed := item.Text != next
	grown := 0
	if changed {
		if c.timelineBytes == nil {
			c.timelineBytes = make(map[string]int)
		}
		if _, ok := c.timelineBytes[sessionKey]; !ok {
			c.timelineBytes[sessionKey] = timelineSize(session.Timeline)
		}
		grown = len(next) - len(item.Text)
		c.timelineBytes[sessionKey] += grown
		item.Text = next
	}
	if buffer.truncated && !session.HistoryTruncated {
		session.HistoryTruncated = true
		changed = true
	}
	item.Streaming = true
	if changed {
		if grown > 4096 || c.timelineBytes[sessionKey] > maxSessionTimelineBytes || len(session.Timeline) > maxSessionTimelineItems {
			c.pruneSessionTimelineTrustedLocked(session)
		}
	}
	return changed
}

// timelineItemIndex returns the current position of a stream buffer's item in
// the session timeline. It trusts the buffer's cached index and repairs it when
// the timeline shifted (for example after retention pruning), so a per-frame
// stream flush never has to rescan the full timeline.
func timelineItemIndex(session *desktopstate.SessionState, buffer *messageStreamBuffer) int {
	if buffer.index >= 0 && buffer.index < len(session.Timeline) {
		if item := session.Timeline[buffer.index]; item.ID == buffer.itemID && item.Kind == buffer.kind {
			return buffer.index
		}
	}
	for index := range session.Timeline {
		if item := session.Timeline[index]; item.ID == buffer.itemID && item.Kind == buffer.kind {
			buffer.index = index
			return index
		}
	}
	buffer.index = -1
	return -1
}

func (c *controller) flushMessageStreamsLocked(sessionID string, agentIDs ...string) {
	for streamKey, buffer := range c.messageStreamBuffers {
		if streamKey.sessionID != sessionID || len(agentIDs) > 0 && agentIDs[0] != "" && streamKey.agentID != agentIDs[0] {
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
	session := desktopstateSessionPointer(&c.state, event.SessionID, event.AgentID)
	if session == nil {
		return
	}
	sessionKey := desktopSessionStorageKey(c.state, session.Ref())
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
		if session := desktopstateSessionPointer(&c.state, event.SessionID, event.AgentID); session != nil && len(session.Subagents) > maxSessionSubagents {
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
	if _, ok := c.timelineBytes[sessionKey]; !ok {
		c.timelineBytes[sessionKey] = timelineSize(session.Timeline)
	}
	var delta int
	switch event.Kind {
	case desktopstate.EventTimelineAppended:
		session.Timeline = append(session.Timeline, event.Item)
		delta = timelineItemSize(event.Item)
	case desktopstate.EventTimelineUpserted:
		delta = upsertTimelineItemLocked(session, event.Item)
	default:
		return
	}
	c.timelineBytes[sessionKey] += delta
	c.pruneSessionTimelineTrustedLocked(session)
}

// upsertTimelineItemLocked merges a timeline item into its existing slot or
// appends it, returning the change in retained size. Unlike a reducer round trip
// it locates, merges, and measures the item in a single pass, and it searches
// the retained tail where streamed and tool items live so a tool-call update is
// usually O(1) instead of rescanning the whole timeline twice.
func upsertTimelineItemLocked(session *desktopstate.SessionState, item desktopstate.TimelineItem) int {
	if item.ID != "" {
		for index := len(session.Timeline) - 1; index >= 0; index-- {
			previous := session.Timeline[index]
			if previous.ID != item.ID || previous.Kind != item.Kind {
				continue
			}
			merged := desktopstate.MergeTimelineItem(previous, item)
			session.Timeline[index] = merged
			return timelineItemSize(merged) - timelineItemSize(previous)
		}
	}
	session.Timeline = append(session.Timeline, item)
	return timelineItemSize(item)
}

func (c *controller) applyStagedHistoryEventsLocked(sessionID string, events []desktopstate.Event, agentIDs ...string) {
	agentID := c.state.ActiveAgentID
	if len(agentIDs) > 0 {
		agentID = agentIDs[0]
	}
	session := desktopstateSessionPointer(&c.state, sessionID, agentID)
	if session == nil {
		return
	}
	sessionKey := desktopSessionStorageKey(c.state, session.Ref())
	if c.timelineBytes == nil {
		c.timelineBytes = make(map[string]int)
	}
	if _, ok := c.timelineBytes[sessionKey]; !ok {
		c.timelineBytes[sessionKey] = timelineSize(session.Timeline)
	}

	appendCount := 0
	for _, event := range events {
		if event.SessionID == sessionID && (event.AgentID == "" || event.AgentID == session.AgentID) && event.Kind == desktopstate.EventTimelineAppended {
			appendCount++
		}
	}
	session.Timeline = slices.Grow(session.Timeline, appendCount)

	for _, event := range events {
		if event.SessionID != sessionID || event.AgentID != "" && event.AgentID != session.AgentID {
			continue
		}
		if event.AgentID == "" {
			event.AgentID = session.AgentID
		}
		if event.Kind != desktopstate.EventTimelineAppended {
			c.applyTimelineEventLocked(event)
			continue
		}
		bounded, truncated, keep := boundSessionEvent(event, maxSessionTimelineBytes)
		if truncated {
			session.HistoryTruncated = true
		}
		if !keep {
			continue
		}
		session.Timeline = append(session.Timeline, bounded.Item)
		c.timelineBytes[sessionKey] += timelineItemSize(bounded.Item)
	}
	// Trim once after the batch rather than rescanning the retained timeline for
	// every appended event, which kept bulk history application quadratic.
	c.pruneSessionTimelineTrustedLocked(session)
}

func timelineSize(items []desktopstate.TimelineItem) int {
	bytes := 0
	for _, item := range items {
		bytes += timelineItemSize(item)
	}
	return bytes
}

// pruneSessionTimelineTrustedLocked trims the session timeline using the
// incrementally maintained byte counter instead of rescanning every retained
// item. Callers guarantee c.timelineBytes[sessionID] reflects the current
// timeline size, which keeps bulk history application linear rather than
// quadratic in the retained item count.
func (c *controller) pruneSessionTimelineTrustedLocked(session *desktopstate.SessionState) {
	sessionKey := desktopSessionStorageKey(c.state, session.Ref())
	if c.timelineBytes[sessionKey] <= maxSessionTimelineBytes && len(session.Timeline) <= maxSessionTimelineItems {
		return
	}
	bytes := c.timelineBytes[sessionKey]
	drop := 0
	for drop < len(session.Timeline) && (bytes > timelineTrimTargetBytes || len(session.Timeline)-drop > timelineTrimTargetItems) {
		if len(session.Timeline)-drop == 1 && bytes > timelineTrimTargetBytes {
			item := &session.Timeline[drop]
			metadata := timelineItemSize(*item) - len(item.Text)
			maxText := timelineTrimTargetBytes - metadata
			if maxText > 0 && len(item.Text) > maxText {
				tail := suffixBytes(item.Text, maxText)
				bytes -= len(item.Text) - len(tail)
				item.Text = tail
				break
			}
		}
		bytes -= timelineItemSize(session.Timeline[drop])
		session.Timeline[drop] = desktopstate.TimelineItem{}
		drop++
	}
	if drop > 0 {
		remaining := append([]desktopstate.TimelineItem(nil), session.Timeline[drop:]...)
		session.Timeline = remaining
	} else if bytes <= maxSessionTimelineBytes && len(session.Timeline) <= maxSessionTimelineItems {
		c.timelineBytes[sessionKey] = bytes
		return
	}
	session.HistoryTruncated = true
	c.timelineBytes[sessionKey] = bytes
}

func retainedTimelineBytes(items []desktopstate.TimelineItem) int {
	bytes := 0
	for _, item := range items {
		bytes += timelineItemSize(item)
	}
	return bytes
}

func desktopstateSessionPointer(state *desktopstate.State, sessionID string, agentIDs ...string) *desktopstate.SessionState {
	for index := range state.Sessions {
		if state.Sessions[index].ID == sessionID && (len(agentIDs) == 0 || agentIDs[0] == "" || state.Sessions[index].AgentID == agentIDs[0]) {
			return &state.Sessions[index]
		}
	}
	return nil
}

func desktopSessionStorageKey(state desktopstate.State, ref desktopstate.SessionRef) string {
	count := 0
	for _, session := range state.Sessions {
		if session.ID == ref.SessionID {
			count++
		}
	}
	if count < 2 || ref.AgentID == "" {
		return ref.SessionID
	}
	return SessionRefStorageKey(ref)
}

func desktopSessionStorageKeyForID(state desktopstate.State, sessionID, agentID string) string {
	if session, ok := SessionByID(state, sessionID, agentID); ok {
		return desktopSessionStorageKey(state, session.Ref())
	}
	return sessionID
}

func hasLiveSessionRef(state desktopstate.State, agentID, sessionID string) bool {
	_, ok := SessionByID(state, sessionID, agentID)
	return ok
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
			AgentID:   update.AgentID,
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
			AgentID:   update.AgentID,
			SessionID: update.SessionID,
			Item: desktopstate.TimelineItem{
				Kind:      itemKind,
				ID:        itemID,
				Text:      update.Text,
				Streaming: true,
			},
		}, true
	case "tool_call", "tool_call_update":
		c.clearMessageStreamsLocked(update.SessionID, update.AgentID)
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
	agentID := c.state.ActiveAgentID
	session, ok := SessionByID(c.state, sessionID, agentID)
	if !ok {
		c.mu.Unlock()
		return
	}
	agentID = session.AgentID
	storageKey := desktopSessionStorageKey(c.state, session.Ref())
	if state := c.histories[storageKey]; state == HistoryStateLoading || state == HistoryStateLoaded {
		c.mu.Unlock()
		return
	}
	workspace := findWorkspaceForSession(c.state, session)
	if workspace != "" {
		if sess := desktopstateSessionPointer(&c.state, sessionID, agentID); sess != nil && sess.Workspace == "" {
			sess.Workspace = workspace
		}
	}
	client, agentID := c.clientForSessionLocked(sessionID, agentID)
	if client == nil || c.connections[agentID] != ConnectionConnected {
		c.mu.Unlock()
		return
	}
	callCtx, cancel := context.WithTimeout(c.context(), historyLoadTimeout)
	request := &sessionHistoryLoad{agentID: agentID, sessionID: sessionID, storageKey: storageKey, cancel: cancel}
	if c.historyLoads == nil {
		c.historyLoads = make(map[string]*sessionHistoryLoad)
	}
	c.historyLoads[storageKey] = request
	c.histories[storageKey] = HistoryStateLoading
	c.historyStaging[storageKey] = nil
	session.HistoryTruncated = false
	if c.timelineBytes == nil {
		c.timelineBytes = make(map[string]int)
	}
	c.timelineBytes[storageKey] = timelineSize(session.Timeline)
	if c.historyStagingBytes == nil {
		c.historyStagingBytes = make(map[string]int)
	}
	c.historyStagingBytes[storageKey] = 0
	delete(c.historyStagingTruncated, storageKey)
	c.clearMessageStreamsLocked(sessionID, agentID)
	c.revision++
	c.mu.Unlock()
	c.notify()
	additionalDirectories := c.additionalDirectoriesForAgent(agentID, session.AdditionalDirectories)
	if workspace == "" {
		additionalDirectories = nil
	}
	params := c.sessionHistoryLoadParams(sessionID, workspace, additionalDirectories)

	c.spawn(func() {
		unlock, acquired := c.lockAgentSessionContext(callCtx, agentID)
		if !acquired {
			c.finishSessionHistoryLoadRequest(client, sessionID, request, callCtx.Err(), nil, nil)
			return
		}
		defer unlock()
		c.mu.Lock()
		currentRequest := c.historyLoads[request.storageKey] == request
		activeSession := isSessionActive(c.state, sessionID, agentID)
		c.mu.Unlock()
		if !currentRequest {
			return
		}
		if !activeSession {
			c.finishSessionHistoryLoadRequest(client, sessionID, request, context.Canceled, nil, nil)
			return
		}
		var loadResult struct {
			ConfigOptions []acpConfigOption `json:"configOptions,omitempty"`
			Models        *acpModelsResult  `json:"models,omitempty"`
			Meta          json.RawMessage   `json:"_meta,omitempty"`
		}
		request.startedAt = time.Now()
		err := client.Call(callCtx, "session/load", params, &loadResult)
		request.serverMeta = loadResult.Meta
		if isACPMethodNotFound(err) {
			err = nil
		}
		c.finishSessionHistoryLoadRequest(client, sessionID, request, err, loadResult.ConfigOptions, loadResult.Models)
	})
}

func (c *controller) sessionHistoryLoadParams(sessionID, workspace string, additionalDirectories []string) map[string]any {
	params := c.mcpSessionParams(sessionID, workspace, additionalDirectories)
	// refreshSessionRuntime fetches the catalog in the background after history loads.
	params["_meta"] = map[string]any{
		"protonman": map[string]any{"deferModelDiscovery": true},
	}
	return params
}

func (c *controller) finishSessionHistoryLoad(client *acpclient.Client, sessionID string, loadErr error) {
	c.finishSessionHistoryLoadRequest(client, sessionID, nil, loadErr, nil, nil)
}

func (c *controller) finishSessionHistoryLoadRequest(client *acpclient.Client, sessionID string, request *sessionHistoryLoad, loadErr error, configOptions []acpConfigOption, modelsResult *acpModelsResult) {
	c.mu.Lock()
	agentID := c.state.ActiveAgentID
	storageKey := desktopSessionStorageKeyForID(c.state, sessionID, agentID)
	if request != nil {
		agentID = request.agentID
		storageKey = request.storageKey
		if c.historyLoads[storageKey] != request {
			c.mu.Unlock()
			return
		}
		delete(c.historyLoads, storageKey)
		request.cancel()
	}
	current, currentAgentID := c.clientForSessionLocked(sessionID, agentID)
	if currentAgentID != "" {
		agentID = currentAgentID
	}
	currentClient := current != nil && current == client
	if !currentClient {
		loadErr = errSessionHistoryClientChanged
	}
	staged := c.historyStaging[storageKey]
	truncated := c.historyStagingTruncated[storageKey]
	delete(c.historyStaging, storageKey)
	delete(c.historyStagingBytes, storageKey)
	delete(c.historyStagingTruncated, storageKey)
	c.clearMessageStreamsLocked(sessionID, agentID)
	active := isSessionActive(c.state, sessionID, agentID)
	applyStaged := loadErr == nil && active
	if applyStaged {
		if session := desktopstateSessionPointer(&c.state, sessionID, agentID); session != nil {
			session.HistoryTruncated = session.HistoryTruncated || truncated
			if models, currentModel := extractModelsFromACP(configOptions, modelsResult); len(models) > 0 {
				c.setAgentAvailableModelsLocked(session.AgentID, models)
				session.AvailableModels = models
				if currentModel != "" && session.Runtime.Model == "" {
					session.Runtime.Model = currentModel
				}
			}
		}
	}
	c.mu.Unlock()
	if applyStaged {
		coalesced := coalesceStagedMessageChunks(staged)
		c.mu.Lock()
		if currentClient && loadErr == nil && isSessionActive(c.state, sessionID, agentID) {
			c.applyStagedHistoryEventsLocked(sessionID, coalesced, agentID)
			c.clearMessageStreamsLocked(sessionID, agentID)
		}
		c.mu.Unlock()
	}
	c.mu.Lock()
	active = isSessionActive(c.state, sessionID, agentID)
	if loadErr == nil && active {
		if session := desktopstateSessionPointer(&c.state, sessionID, agentID); session != nil {
			session.HistoryTruncated = session.HistoryTruncated || truncated
			if models, currentModel := extractModelsFromACP(configOptions, modelsResult); len(models) > 0 {
				if !applyStaged {
					c.setAgentAvailableModelsLocked(session.AgentID, models)
					session.AvailableModels = models
					if currentModel != "" && session.Runtime.Model == "" {
						session.Runtime.Model = currentModel
					}
				}
			}
		}
		c.histories[storageKey] = HistoryStateLoaded
		if truncated {
			c.statuses[agentID] = "Connected · recent session history loaded"
		} else {
			c.statuses[agentID] = "Connected · session history loaded"
		}
	} else if loadErr == nil || errors.Is(loadErr, context.Canceled) {
		c.histories[storageKey] = HistoryStateUnloaded
	} else {
		c.histories[storageKey] = HistoryStateUnloaded
		if active {
			c.statuses[agentID] = "Session history failed · " + compactError(loadErr)
		}
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
	logSessionLoadTiming(sessionID, request, loadErr)
	c.mu.RLock()
	stillActive := isSessionActive(c.state, sessionID, agentID)
	c.mu.RUnlock()
	if currentClient && loadErr == nil && stillActive {
		c.refreshSessionContextFrom(client, sessionID, false, agentID)
		c.refreshSessionMemoryFrom(client, sessionID, false, agentID)
		c.refreshSessionRuntime(sessionID, false, agentID)
	}
}

func (c *controller) resumeKnownSessions(agentID string, client *acpclient.Client) {
	if !c.agentSupportsSessionResume(agentID) {
		return
	}
	c.mu.RLock()
	sessions := append([]desktopstate.SessionState(nil), c.state.Sessions...)
	activeSessionID := c.state.ActiveSessionID
	c.mu.RUnlock()
	for _, session := range sessions {
		if c.ctx.Err() != nil {
			return
		}
		workspace := strings.TrimSpace(session.Workspace)
		if workspace == "" {
			if defaultWS, err := newSessionWorkspace(c.state); err == nil && defaultWS != "" {
				wsKey := sessionWorkspaceKey(defaultWS)
				sessKey := sessionWorkspaceKeyFromSession(session)
				if sessKey == "" || sessKey == wsKey || sessKey == "workspace:"+canonicalWorkspacePath(defaultWS) {
					workspace = defaultWS
					c.mu.Lock()
					if sess := desktopstateSessionPointer(&c.state, session.ID); sess != nil {
						sess.Workspace = defaultWS
					}
					c.mu.Unlock()
				}
			}
		}
		if session.ID == "" || workspace == "" || session.AgentID != agentID && !(session.AgentID == "" && agentID == ProtonmanAgentID) {
			continue
		}
		if sessKey := sessionWorkspaceKeyFromSession(session); sessKey != "" {
			if sessionWorkspaceKey(workspace) != sessKey {
				continue
			}
		}
		additionalDirectories := c.additionalDirectoriesForAgent(agentID, session.AdditionalDirectories)
		params := c.mcpSessionParams(session.ID, workspace, additionalDirectories)
		unlock, acquired := c.lockAgentSessionContext(c.context(), agentID)
		if !acquired {
			return
		}
		callCtx, cancel := context.WithTimeout(c.context(), reconnectRequestTimeout)
		var resumeResult struct {
			Modes *struct {
				CurrentModeID string `json:"currentModeId"`
			} `json:"modes,omitempty"`
			ConfigOptions []acpConfigOption `json:"configOptions"`
			Models        *acpModelsResult  `json:"models,omitempty"`
		}
		err := client.Call(callCtx, "session/resume", params, &resumeResult)
		cancel()
		unlock()
		if err != nil && !isACPMethodNotFound(err) && c.context().Err() == nil {
			if session.ID == activeSessionID {
				c.setAgentStatus(agentID, "Session resume failed · "+compactError(err))
			}
		} else if err == nil {
			if models, currentModel := extractModelsFromACP(resumeResult.ConfigOptions, resumeResult.Models); len(models) > 0 {
				c.mu.Lock()
				c.setAgentAvailableModelsLocked(agentID, models)
				if sess := desktopstateSessionPointer(&c.state, session.ID); sess != nil {
					sess.AvailableModels = models
					if currentModel != "" && sess.Runtime.Model == "" {
						sess.Runtime.Model = currentModel
					}
					c.revision++
				}
				c.mu.Unlock()
				c.notify()
			}
			if resumeResult.Modes != nil && c.isClineAgent(agentID) {
				c.setClinePermissionModeFromACP(agentID, session.ID, resumeResult.Modes.CurrentModeID, resumeResult.ConfigOptions)
			}
		}
	}
}

func (c *controller) sendPrompt(text string) {
	c.SendExpandedPrompt(ExpandedPrompt{DisplayText: text, TurnPrompt: text})
}

func (c *controller) SendExpandedPrompt(prompt ExpandedPrompt) {
	text := strings.TrimSpace(prompt.DisplayText)
	if text == "" {
		return
	}
	mcpServers := c.mcpServersPayload()
	c.mu.Lock()
	sessionID := c.state.ActiveSessionID
	agentID := c.state.ActiveAgentID
	session, ok := SessionByID(c.state, sessionID, agentID)
	workspace := strings.TrimSpace(session.Workspace)
	if workspace == "" {
		if defaultWS, err := newSessionWorkspace(c.state); err == nil && defaultWS != "" {
			workspace = defaultWS
			if current := desktopstateSessionPointer(&c.state, sessionID, agentID); current != nil {
				current.Workspace = defaultWS
			}
		}
	}
	client, agentID := c.clientForSessionLocked(sessionID, agentID)
	storageKey := desktopSessionStorageKey(c.state, session.Ref())
	if !ok || client == nil || c.connections[agentID] != ConnectionConnected || SessionBusy(session.Status) || c.histories[storageKey] == HistoryStateLoading || workspace == "" {
		c.mu.Unlock()
		return
	}
	c.clearMessageStreamsLocked(sessionID)
	if current := desktopstateSessionPointer(&c.state, sessionID, agentID); current != nil {
		current.LastActivityAt = time.Now().UTC()
	}
	desktopstate.Apply(&c.state, desktopstate.Event{Kind: desktopstate.EventPromptStarted, AgentID: agentID, SessionID: sessionID})
	c.applyTimelineEventLocked(desktopstate.Event{
		Kind:      desktopstate.EventTimelineAppended,
		AgentID:   agentID,
		SessionID: sessionID,
		Item: desktopstate.TimelineItem{
			Kind: desktopstate.TimelineUser,
			ID:   c.nextTimelineIDLocked(sessionID, "prompt"),
			Text: prompt.DisplayText,
		},
	})
	c.statuses[agentID] = "Running · " + session.Title
	c.revision++
	c.mu.Unlock()
	c.notify()

	c.spawn(func() { c.runPrompt(client, session, prompt, mcpServers) })
}

func (c *controller) runPrompt(client *acpclient.Client, session desktopstate.SessionState, prompt ExpandedPrompt, mcpServers []map[string]any) {
	defer c.lockAgentSession(session.AgentID)()
	params := map[string]any{"sessionId": session.ID, "cwd": session.Workspace}
	if additionalDirectories := c.additionalDirectoriesForAgent(session.AgentID, session.AdditionalDirectories); len(additionalDirectories) > 0 {
		params["additionalDirectories"] = additionalDirectories
	}
	if len(mcpServers) > 0 {
		params["mcpServers"] = mcpServers
	}
	var err error
	if c.agentSupportsSessionResume(session.AgentID) {
		err = client.Call(c.context(), "session/resume", params, nil)
	}
	var result struct {
		StopReason string `json:"stopReason"`
	}
	if err == nil {
		promptBlocks := []map[string]any{
			{"type": "text", "text": prompt.TurnPrompt},
		}
		for _, imgPath := range prompt.ImagePaths {
			imgBytes, readErr := os.ReadFile(imgPath)
			if readErr != nil {
				continue
			}
			promptBlocks = append(promptBlocks, map[string]any{
				"type":     "image",
				"mimeType": detectImageMIME(imgPath),
				"data":     base64.StdEncoding.EncodeToString(imgBytes),
			})
		}
		promptParams := map[string]any{
			"sessionId": session.ID,
			"prompt":    promptBlocks,
		}
		c.mu.RLock()
		profile := c.profiles[session.AgentID]
		authMethods := slices.Clone(c.agentFeatures[session.AgentID].AuthMethods)
		c.mu.RUnlock()
		err = promptACPWithReauthentication(c.context(), client, profile, authMethods, promptParams, &result, func(methodName string) {
			name := strings.TrimSpace(methodName)
			if name == "" {
				name = "Cline"
			}
			c.setAgentStatus(session.AgentID, "Re-authenticating · "+name)
		})
	}

	c.mu.Lock()
	if !c.clientCurrentLocked(session.AgentID, client) {
		c.mu.Unlock()
		return
	}
	c.clearMessageStreamsLocked(session.ID)
	if err != nil {
		desktopstate.Apply(&c.state, desktopstate.Event{Kind: desktopstate.EventPromptFailed, AgentID: session.AgentID, SessionID: session.ID})
		c.applyTimelineEventLocked(desktopstate.Event{
			Kind:      desktopstate.EventTimelineAppended,
			AgentID:   session.AgentID,
			SessionID: session.ID,
			Item: desktopstate.TimelineItem{
				Kind: desktopstate.TimelineStatus,
				ID:   c.nextTimelineIDLocked(session.ID, "error"),
				Text: "Prompt failed: " + compactError(err),
			},
		})
		c.statuses[session.AgentID] = "Prompt failed · " + compactError(err)
	} else {
		desktopstate.Apply(&c.state, desktopstate.Event{Kind: desktopstate.EventPromptCompleted, AgentID: session.AgentID, SessionID: session.ID})
		stopReason := strings.TrimSpace(result.StopReason)
		if stopReason == "" {
			stopReason = "completed"
		}
		c.statuses[session.AgentID] = "Connected · " + stopReason
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
	c.refreshSessionContextFrom(client, session.ID, false, session.AgentID)
	c.refreshSessionMemoryFrom(client, session.ID, false, session.AgentID)
	c.refreshSessionRuntime(session.ID, false, session.AgentID)
}

func (c *controller) CancelPrompt() {
	c.mu.RLock()
	sessionID := c.state.ActiveSessionID
	session, ok := SessionByID(c.state, sessionID, c.state.ActiveAgentID)
	client, agentID := c.clientForSessionLocked(sessionID, c.state.ActiveAgentID)
	c.mu.RUnlock()
	if !ok || client == nil || !SessionBusy(session.Status) {
		return
	}
	c.setAgentStatus(agentID, "Cancelling…")
	c.spawn(func() {
		callCtx, cancel := context.WithTimeout(c.context(), reconnectRequestTimeout)
		defer cancel()
		if err := client.Call(callCtx, "session/cancel", map[string]any{"sessionId": sessionID}, nil); err != nil && c.context().Err() == nil {
			c.mu.Lock()
			if c.clients[agentID] == client {
				c.statuses[agentID] = "Cancel failed · " + compactError(err)
				c.revision++
			}
			c.mu.Unlock()
			c.notify()
		}
	})
}

func (c *controller) pruneSessionRuntimeLocked() {
	live := make(map[string]struct{}, len(c.state.Sessions))
	for _, session := range c.state.Sessions {
		live[session.ID] = struct{}{}
		live[SessionRefStorageKey(session.Ref())] = struct{}{}
		live[desktopSessionStorageKey(c.state, session.Ref())] = struct{}{}
	}
	for sessionID := range c.histories {
		if _, ok := live[sessionID]; !ok {
			delete(c.histories, sessionID)
		}
	}
	for sessionID := range c.timelineBytes {
		if _, ok := live[sessionID]; !ok {
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
	for sessionID := range c.historyStagingBytes {
		if _, ok := live[sessionID]; !ok {
			delete(c.historyStagingBytes, sessionID)
		}
	}
	for sessionID := range c.messageSequence {
		if _, ok := live[sessionID]; !ok {
			delete(c.messageSequence, sessionID)
		}
	}
	streamIsLive := func(key messageStreamKey) bool {
		if key.agentID == "" {
			_, ok := live[key.sessionID]
			return ok
		}
		return hasLiveSessionRef(c.state, key.agentID, key.sessionID)
	}
	for streamKey, buffer := range c.messageStreamBuffers {
		if !streamIsLive(streamKey) {
			if buffer != nil && buffer.timer != nil {
				buffer.timer.Stop()
			}
			delete(c.messageStreamBuffers, streamKey)
			delete(c.messageStreams, streamKey)
		}
	}
	for streamKey := range c.messageStreams {
		if !streamIsLive(streamKey) {
			delete(c.messageStreams, streamKey)
		}
	}
	permissions := c.state.PermissionInbox[:0]
	livePermissionRequests := make(map[string]struct{}, len(c.state.PermissionInbox))
	for _, permission := range c.state.PermissionInbox {
		if _, ok := live[permission.SessionID]; ok && (permission.AgentID == "" || hasLiveSessionRef(c.state, permission.AgentID, permission.SessionID)) {
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
	questions := c.state.QuestionInbox[:0]
	liveQuestionRequests := make(map[string]struct{}, len(c.state.QuestionInbox))
	for _, question := range c.state.QuestionInbox {
		if _, ok := live[question.SessionID]; ok && (question.AgentID == "" || hasLiveSessionRef(c.state, question.AgentID, question.SessionID)) {
			questions = append(questions, question)
			liveQuestionRequests[question.RequestID] = struct{}{}
		}
	}
	c.state.QuestionInbox = questions
	for requestID, waiter := range c.questionWait {
		if _, ok := liveQuestionRequests[requestID]; !ok {
			select {
			case waiter <- desktopstate.QuestionResponse{Status: "declined", Answer: "session ended"}:
			default:
			}
			delete(c.questionWait, requestID)
		}
	}
	if _, ok := live[c.runtimeMutation]; !ok {
		c.runtimeMutation = ""
	}
	c.pruneAgentRuntimeLocked()
}

// pruneInactiveSessionHistoryLocked keeps the active and immediately previous
// loaded idle transcripts resident. Inactive running sessions must reload because
// their updates are not applied while another session is selected.
func (c *controller) pruneInactiveSessionHistoryLocked(activeSessionID, previousSessionID string, refs ...desktopstate.SessionRef) {
	activeAgentID, previousAgentID := "", ""
	if len(refs) > 0 {
		activeAgentID = refs[0].AgentID
	}
	if len(refs) > 1 {
		previousAgentID = refs[1].AgentID
	}
	for index := range c.state.Sessions {
		session := &c.state.Sessions[index]
		if session.ID == activeSessionID && (activeAgentID == "" || session.AgentID == activeAgentID) {
			continue
		}
		sessionKey := desktopSessionStorageKey(c.state, session.Ref())
		if load := c.historyLoads[sessionKey]; load != nil {
			load.cancel()
			delete(c.historyLoads, sessionKey)
			c.histories[sessionKey] = HistoryStateUnloaded
		}
		delete(c.historyStaging, sessionKey)
		delete(c.historyStagingBytes, sessionKey)
		delete(c.historyStagingTruncated, sessionKey)
		for streamKey := range c.messageStreams {
			if streamKey.sessionID == session.ID && (streamKey.agentID == "" || streamKey.agentID == session.AgentID) {
				if buffer := c.messageStreamBuffers[streamKey]; buffer != nil && buffer.timer != nil {
					buffer.timer.Stop()
				}
				delete(c.messageStreamBuffers, streamKey)
				delete(c.messageStreams, streamKey)
			}
		}
		if session.ID == previousSessionID && (previousAgentID == "" || session.AgentID == previousAgentID) && c.histories[sessionKey] == HistoryStateLoaded && !SessionBusy(session.Status) {
			continue
		}
		session.Timeline = nil
		session.HistoryTruncated = false
		session.Subagents = nil
		session.Context = desktopstate.SessionContextState{}
		session.Runtime = desktopstate.RuntimeSettingsState{}
		delete(c.timelineBytes, sessionKey)
		c.histories[sessionKey] = HistoryStateUnloaded
	}
}

func (c *controller) resetAgentTransientSessionStateLocked(agentID string) {
	for _, session := range c.state.Sessions {
		if session.AgentID != agentID {
			continue
		}
		sessionKey := desktopSessionStorageKey(c.state, session.Ref())
		if load := c.historyLoads[sessionKey]; load != nil {
			load.cancel()
			delete(c.historyLoads, sessionKey)
			c.histories[sessionKey] = HistoryStateUnloaded
		}
		if c.histories[sessionKey] == HistoryStateLoading {
			c.histories[sessionKey] = HistoryStateUnloaded
		}
		delete(c.historyStaging, sessionKey)
		delete(c.historyStagingBytes, sessionKey)
		delete(c.historyStagingTruncated, sessionKey)
		c.clearMessageStreamsLocked(session.ID, session.AgentID)
		for _, permission := range c.state.PermissionInbox {
			if permission.SessionID != session.ID || permission.AgentID != "" && permission.AgentID != agentID {
				continue
			}
			delete(c.permissionWait, permission.RequestID)
		}
		if c.runtimeMutation == session.ID || c.runtimeMutation == SessionRefStorageKey(session.Ref()) {
			c.runtimeMutation = ""
		}
	}
}

func (c *controller) nextTimelineIDLocked(sessionID, prefix string) string {
	c.messageSequence[sessionID]++
	return fmt.Sprintf("%s-%d", prefix, c.messageSequence[sessionID])
}

func (c *controller) clearMessageStreamsLocked(sessionID string, agentIDs ...string) {
	agentID := ""
	if len(agentIDs) > 0 {
		agentID = agentIDs[0]
	}
	session := desktopstateSessionPointer(&c.state, sessionID, agentID)
	for streamKey := range c.messageStreams {
		if streamKey.sessionID != sessionID || agentID != "" && streamKey.agentID != "" && streamKey.agentID != agentID {
			continue
		}
		itemID := c.messageStreams[streamKey]
		buffer := c.messageStreamBuffers[streamKey]
		if buffer != nil {
			if buffer.timer != nil {
				buffer.timer.Stop()
				buffer.timer = nil
			}
			c.flushMessageStreamLocked(buffer)
			delete(c.messageStreamBuffers, streamKey)
		}
		if session != nil {
			index := -1
			if buffer != nil {
				index = timelineItemIndex(session, buffer)
			}
			if index < 0 {
				for candidate := range session.Timeline {
					if session.Timeline[candidate].ID == itemID {
						index = candidate
						break
					}
				}
			}
			if index >= 0 {
				session.Timeline[index].Streaming = false
			}
		}
		delete(c.messageStreams, streamKey)
	}
}

func sessionIndex(sessions []desktopstate.SessionState, sessionID string, agentIDs ...string) (int, bool) {
	for index := range sessions {
		if sessions[index].ID == sessionID && (len(agentIDs) == 0 || agentIDs[0] == "" || sessions[index].AgentID == agentIDs[0]) {
			return index, true
		}
	}
	return 0, false
}

// SessionByID resolves a session by ID, optionally narrowed to one owning agent.
func SessionByID(state desktopstate.State, sessionID string, agentIDs ...string) (desktopstate.SessionState, bool) {
	for _, session := range state.Sessions {
		if session.ID == sessionID && (len(agentIDs) == 0 || agentIDs[0] == "" || session.AgentID == agentIDs[0]) {
			return session, true
		}
	}
	return desktopstate.SessionState{}, false
}

func isSessionActive(state desktopstate.State, sessionID, agentID string) bool {
	if state.ActiveSessionID != sessionID || sessionID == "" {
		return false
	}
	if state.ActiveAgentID == "" || state.ActiveAgentID == agentID {
		return true
	}
	if (state.ActiveAgentID == ProtonmanAgentID && (agentID == "" || agentID == ProtonmanAgentID)) ||
		(state.ActiveAgentID == "" && (agentID == "" || agentID == ProtonmanAgentID)) {
		return true
	}
	return false
}

func findWorkspaceForSession(state desktopstate.State, session desktopstate.SessionState) string {
	if ws := strings.TrimSpace(session.Workspace); ws != "" {
		return ws
	}
	sessKey := sessionWorkspaceKeyFromSession(session)
	for _, project := range state.Projects {
		for _, folder := range project.Folders {
			folderPath := strings.TrimSpace(folder.Path)
			if folderPath == "" {
				continue
			}
			if sessKey != "" && (sessionWorkspaceKey(folderPath) == sessKey || sessionWorkspaceKey(canonicalWorkspacePath(folderPath)) == sessKey) {
				return folderPath
			}
		}
	}
	if defaultWS, err := newSessionWorkspace(state); err == nil && defaultWS != "" {
		wsKey := sessionWorkspaceKey(defaultWS)
		if sessKey == "" || sessKey == wsKey || sessKey == "workspace:"+canonicalWorkspacePath(defaultWS) || (sessKey != "" && sessionWorkspaceKey(canonicalWorkspacePath(defaultWS)) == sessKey) {
			return defaultWS
		}
	}
	return ""
}

// SessionBusy reports whether a session still has work in flight, so runtime
// mutation and prompt submission must stay disabled until it settles.
func SessionBusy(status desktopstate.TaskStatus) bool {
	switch status {
	case desktopstate.TaskQueued, desktopstate.TaskRunning, desktopstate.TaskWaitingPermission, desktopstate.TaskWaitingUser:
		return true
	default:
		return false
	}
}

// desktopTimingEnabled reports whether diagnostic stage timings are requested.
// The switch is shared with the ACP agent through PROTONMAN_TIMING so a single
// environment variable enables timings on both ends of the pipe.
func desktopTimingEnabled() bool {
	return envconfig.Bool(envconfig.Timing)
}

// acpLoadTimings mirrors the `_meta.protonman.timings` payload the agent attaches
// to session/load when timing is enabled. Values are microseconds.
type acpLoadTimings struct {
	Protonman struct {
		Timings struct {
			NewSessionUs    int64 `json:"newSessionUs"`
			DiskLoadUs      int64 `json:"diskLoadUs"`
			RestoreUs       int64 `json:"restoreUs"`
			ReplayUs        int64 `json:"replayUs"`
			ReplayMessages  int   `json:"replayMessages"`
			ConfigOptionsUs int64 `json:"configOptionsUs"`
			SessionLoadUs   int64 `json:"sessionLoadUs"`
			TotalUs         int64 `json:"totalUs"`
		} `json:"timings"`
	} `json:"protonman"`
}

// logSessionLoadTiming reports one diagnostic line for a completed history load.
// It is a no-op unless PROTONMAN_TIMING is set, so normal runs stay quiet.
func logSessionLoadTiming(sessionID string, request *sessionHistoryLoad, loadErr error) {
	if !desktopTimingEnabled() {
		return
	}
	var clientElapsed time.Duration
	var meta json.RawMessage
	if request != nil {
		meta = request.serverMeta
		if !request.startedAt.IsZero() {
			clientElapsed = time.Since(request.startedAt)
		}
	}
	var parsed acpLoadTimings
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &parsed)
	}
	t := parsed.Protonman.Timings
	micro := func(value int64) time.Duration { return time.Duration(value) * time.Microsecond }
	log.Printf(
		"[TIMING] session/load session=%s err=%v client=%s server.total=%s server.sessionLoad=%s newSession=%s diskLoad=%s restore=%s replay=%s(%d messages) configOptions=%s",
		sessionID,
		loadErr,
		clientElapsed.Round(time.Millisecond),
		micro(t.TotalUs).Round(time.Millisecond),
		micro(t.SessionLoadUs).Round(time.Millisecond),
		micro(t.NewSessionUs).Round(time.Millisecond),
		micro(t.DiskLoadUs).Round(time.Millisecond),
		micro(t.RestoreUs).Round(time.Millisecond),
		micro(t.ReplayUs).Round(time.Millisecond),
		t.ReplayMessages,
		micro(t.ConfigOptionsUs).Round(time.Millisecond),
	)
}
