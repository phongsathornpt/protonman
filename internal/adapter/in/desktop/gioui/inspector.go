//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	contextRefreshInterval  = 750 * time.Millisecond
	memoryRefreshInterval   = 3 * time.Second
	runtimeRefreshInterval  = time.Second
	skillsRefreshInterval   = 3 * time.Second
	inspectorRequestTimeout = 15 * time.Second
)

type sessionRefreshTracker struct {
	mu       sync.Mutex
	interval time.Duration
	last     map[string]time.Time
	inFlight map[string]bool
}

func newSessionRefreshTracker(interval time.Duration) *sessionRefreshTracker {
	return &sessionRefreshTracker{
		interval: interval,
		last:     make(map[string]time.Time),
		inFlight: make(map[string]bool),
	}
}

func (t *sessionRefreshTracker) begin(sessionID string, force bool, now time.Time) bool {
	if t == nil || sessionID == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == nil {
		t.last = make(map[string]time.Time)
	}
	if t.inFlight == nil {
		t.inFlight = make(map[string]bool)
	}
	if t.inFlight[sessionID] {
		return false
	}
	if !force {
		if last := t.last[sessionID]; !last.IsZero() && now.Sub(last) < t.interval {
			return false
		}
	}
	t.inFlight[sessionID] = true
	return true
}

func (t *sessionRefreshTracker) finish(sessionID string, now time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.inFlight[sessionID]; !ok {
		return
	}
	delete(t.inFlight, sessionID)
	t.last[sessionID] = now
}

func (t *sessionRefreshTracker) prune(live map[string]struct{}) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for sessionID := range t.last {
		if _, ok := live[sessionID]; !ok {
			delete(t.last, sessionID)
		}
	}
	for sessionID := range t.inFlight {
		if _, ok := live[sessionID]; !ok {
			delete(t.inFlight, sessionID)
		}
	}
}

func (t *sessionRefreshTracker) reset() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	clear(t.last)
	clear(t.inFlight)
}

type sessionContextResult struct {
	SessionID string `json:"sessionId"`
	Goal      string `json:"goal"`
	Todo      struct {
		Revision uint64 `json:"revision"`
		Items    []struct {
			ID     string `json:"id"`
			Text   string `json:"text"`
			Status string `json:"status"`
		} `json:"items"`
	} `json:"todo"`
}

type sessionMemoryResult struct {
	SessionID    string `json:"sessionId"`
	WorkspaceKey string `json:"workspaceKey"`
	Workspace    []struct {
		ID         string  `json:"id"`
		Scope      string  `json:"scope"`
		Kind       string  `json:"kind"`
		Key        string  `json:"key"`
		Value      string  `json:"value"`
		Confidence float64 `json:"confidence"`
		UsageCount uint64  `json:"usageCount"`
	} `json:"workspace"`
	Global []struct {
		ID         string  `json:"id"`
		Scope      string  `json:"scope"`
		Kind       string  `json:"kind"`
		Key        string  `json:"key"`
		Value      string  `json:"value"`
		Confidence float64 `json:"confidence"`
		UsageCount uint64  `json:"usageCount"`
	} `json:"global"`
}

type sessionRuntimeResult struct {
	SessionID       string          `json:"sessionId"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model"`
	Reasoning       string          `json:"reasoning"`
	LowConcurrency  string          `json:"lowConcurrency"`
	AvailableModels json.RawMessage `json:"availableModels,omitempty"`
}

type sessionSkillsResult struct {
	SessionID string `json:"sessionId"`
	Skills    []struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Scope       string   `json:"scope"`
		Active      bool     `json:"active"`
		Locked      bool     `json:"locked"`
		LockStatus  string   `json:"lockStatus"`
		Resources   []string `json:"resources"`
	} `json:"skills"`
}

type sessionRefreshKind uint8

const (
	contextRefreshKind sessionRefreshKind = iota
	memoryRefreshKind
	runtimeRefreshKind
	skillsRefreshKind
)

func refreshIntervalFor(kind sessionRefreshKind) time.Duration {
	switch kind {
	case contextRefreshKind:
		return contextRefreshInterval
	case memoryRefreshKind:
		return memoryRefreshInterval
	case skillsRefreshKind:
		return skillsRefreshInterval
	default:
		return runtimeRefreshInterval
	}
}

func (c *controller) beginSessionRefresh(sessionID string, force bool, kind sessionRefreshKind, source *acpclient.Client) (*acpclient.Client, *sessionRefreshTracker, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	session, ok := desktopSessionByID(c.state, sessionID)
	if !ok || sessionID != c.state.ActiveSessionID || session.AgentID != controllerAgentID {
		return nil, nil, false
	}
	client, agentID := c.clientForSessionLocked(sessionID)
	if client == nil || c.connections[agentID] != connectionConnected || source != nil && !c.clientCurrentLocked(agentID, source) {
		return nil, nil, false
	}
	var tracker *sessionRefreshTracker
	switch kind {
	case contextRefreshKind:
		tracker = c.contextRefresh
		if tracker == nil {
			tracker = newSessionRefreshTracker(refreshIntervalFor(kind))
			c.contextRefresh = tracker
		}
	case memoryRefreshKind:
		tracker = c.memoryRefresh
		if tracker == nil {
			tracker = newSessionRefreshTracker(refreshIntervalFor(kind))
			c.memoryRefresh = tracker
		}
	case skillsRefreshKind:
		tracker = c.skillsRefresh
		if tracker == nil {
			tracker = newSessionRefreshTracker(refreshIntervalFor(kind))
			c.skillsRefresh = tracker
		}
	default:
		tracker = c.runtimeRefresh
		if tracker == nil {
			tracker = newSessionRefreshTracker(refreshIntervalFor(kind))
			c.runtimeRefresh = tracker
		}
	}
	if !tracker.begin(sessionID, force, time.Now()) {
		return nil, tracker, false
	}
	return client, tracker, true
}

func (c *controller) refreshSessionContext(sessionID string, force bool) {
	c.refreshSessionContextFrom(nil, sessionID, force)
}

func (c *controller) refreshSessionContextFrom(source *acpclient.Client, sessionID string, force bool) {
	sessionID = strings.TrimSpace(sessionID)
	client, tracker, ok := c.beginSessionRefresh(sessionID, force, contextRefreshKind, source)
	if !ok {
		return
	}
	go func() {
		defer tracker.finish(sessionID, time.Now())
		var result sessionContextResult
		if err := c.callInspector(client, "protonman/session/context", map[string]any{"sessionId": sessionID}, &result); err != nil {
			return
		}
		c.applySessionContext(client, result)
	}()
}

func (c *controller) refreshSessionMemory(sessionID string, force bool) {
	c.refreshSessionMemoryFrom(nil, sessionID, force)
}

func (c *controller) refreshSessionMemoryFrom(source *acpclient.Client, sessionID string, force bool) {
	sessionID = strings.TrimSpace(sessionID)
	client, tracker, ok := c.beginSessionRefresh(sessionID, force, memoryRefreshKind, source)
	if !ok {
		return
	}
	go func() {
		defer tracker.finish(sessionID, time.Now())
		var result sessionMemoryResult
		if err := c.callInspector(client, "protonman/session/memory", map[string]any{"sessionId": sessionID}, &result); err != nil {
			return
		}
		c.applySessionMemory(client, result)
	}()
}

func (c *controller) refreshSessionRuntime(sessionID string, force bool) {
	sessionID = strings.TrimSpace(sessionID)
	client, tracker, ok := c.beginSessionRefresh(sessionID, force, runtimeRefreshKind, nil)
	if !ok {
		return
	}
	go func() {
		defer tracker.finish(sessionID, time.Now())
		var result sessionRuntimeResult
		if err := c.callInspector(client, "protonman/session/runtime", map[string]any{"sessionId": sessionID}, &result); err != nil {
			return
		}
		c.applySessionRuntime(client, result)
	}()
}

func (c *controller) refreshSessionSkills(sessionID string, force bool) {
	c.refreshSessionSkillsFrom(nil, sessionID, force)
}

func (c *controller) refreshSessionSkillsFrom(source *acpclient.Client, sessionID string, force bool) {
	sessionID = strings.TrimSpace(sessionID)
	client, tracker, ok := c.beginSessionRefresh(sessionID, force, skillsRefreshKind, source)
	if !ok {
		return
	}
	go func() {
		defer tracker.finish(sessionID, time.Now())
		var result sessionSkillsResult
		if err := c.callInspector(client, "protonman/session/skills", map[string]any{"sessionId": sessionID}, &result); err != nil {
			return
		}
		c.applySessionSkills(client, result)
	}()
}

func (c *controller) refreshActiveSession(force bool) {
	c.mu.RLock()
	sessionID := c.state.ActiveSessionID
	c.mu.RUnlock()
	c.refreshSessionContext(sessionID, force)
	c.refreshSessionMemory(sessionID, force)
	c.refreshSessionRuntime(sessionID, force)
	c.refreshSessionSkills(sessionID, force)
}

func (c *controller) callInspector(client *acpclient.Client, method string, params any, result any) error {
	base := c.ctx
	if base == nil {
		base = context.Background()
	}
	callCtx, cancel := context.WithTimeout(base, inspectorRequestTimeout)
	defer cancel()
	return client.Call(callCtx, method, params, result)
}

func projectSessionContext(result sessionContextResult) desktopstate.SessionContextState {
	items := make([]desktopstate.TodoItemState, 0, len(result.Todo.Items))
	for _, item := range result.Todo.Items {
		items = append(items, desktopstate.TodoItemState{
			ID:     strings.TrimSpace(item.ID),
			Text:   strings.TrimSpace(item.Text),
			Status: strings.TrimSpace(item.Status),
		})
	}
	return desktopstate.SessionContextState{
		Goal: strings.TrimSpace(result.Goal),
		Todo: desktopstate.TodoState{Revision: result.Todo.Revision, Items: items},
	}
}

func projectSessionMemory(result sessionMemoryResult) desktopstate.MemoryState {
	memory := desktopstate.MemoryState{WorkspaceKey: strings.TrimSpace(result.WorkspaceKey)}
	memory.Workspace = make([]desktopstate.MemoryEntryState, 0, len(result.Workspace))
	for _, item := range result.Workspace {
		memory.Workspace = append(memory.Workspace, projectMemoryEntry(item.ID, item.Scope, item.Kind, item.Key, item.Value, item.Confidence, item.UsageCount))
	}
	memory.Global = make([]desktopstate.MemoryEntryState, 0, len(result.Global))
	for _, item := range result.Global {
		memory.Global = append(memory.Global, projectMemoryEntry(item.ID, item.Scope, item.Kind, item.Key, item.Value, item.Confidence, item.UsageCount))
	}
	return memory
}

func projectMemoryEntry(id, scope, kind, key, value string, confidence float64, usageCount uint64) desktopstate.MemoryEntryState {
	return desktopstate.MemoryEntryState{
		ID:         strings.TrimSpace(id),
		Scope:      strings.TrimSpace(scope),
		Kind:       strings.TrimSpace(kind),
		Key:        strings.TrimSpace(key),
		Value:      strings.TrimSpace(value),
		Confidence: confidence,
		UsageCount: usageCount,
	}
}

func projectSessionRuntime(result sessionRuntimeResult) desktopstate.RuntimeSettingsState {
	return desktopstate.RuntimeSettingsState{
		Provider:       strings.TrimSpace(result.Provider),
		Model:          strings.TrimSpace(result.Model),
		Reasoning:      strings.TrimSpace(result.Reasoning),
		LowConcurrency: strings.TrimSpace(result.LowConcurrency),
	}
}

func (c *controller) applySessionContext(client *acpclient.Client, result sessionContextResult) bool {
	sessionID := strings.TrimSpace(result.SessionID)
	if client == nil || sessionID == "" {
		return false
	}
	c.mu.Lock()
	session, ok := desktopSessionByID(c.state, sessionID)
	if !ok || sessionID != c.state.ActiveSessionID || !c.clientCurrentLocked(session.AgentID, client) {
		c.mu.Unlock()
		return false
	}
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventSessionContextUpdated,
		SessionID: sessionID,
		Context:   projectSessionContext(result),
	})
	c.revision++
	c.mu.Unlock()
	c.notify()
	return true
}

func (c *controller) applySessionMemory(client *acpclient.Client, result sessionMemoryResult) bool {
	sessionID := strings.TrimSpace(result.SessionID)
	if client == nil || sessionID == "" {
		return false
	}
	c.mu.Lock()
	session, ok := desktopSessionByID(c.state, sessionID)
	if !ok || sessionID != c.state.ActiveSessionID || !c.clientCurrentLocked(session.AgentID, client) {
		c.mu.Unlock()
		return false
	}
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventSessionMemoryUpdated,
		SessionID: sessionID,
		Memory:    projectSessionMemory(result),
	})
	c.revision++
	c.mu.Unlock()
	c.notify()
	return true
}

func (c *controller) applySessionRuntime(client *acpclient.Client, result sessionRuntimeResult) bool {
	sessionID := strings.TrimSpace(result.SessionID)
	if client == nil || sessionID == "" {
		return false
	}
	c.mu.Lock()
	session, ok := desktopSessionByID(c.state, sessionID)
	if !ok || sessionID != c.state.ActiveSessionID || !c.clientCurrentLocked(session.AgentID, client) {
		c.mu.Unlock()
		return false
	}
	models := decodeAvailableModels(result.AvailableModels)
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:            desktopstate.EventSessionRuntimeUpdated,
		SessionID:       sessionID,
		Runtime:         projectSessionRuntime(result),
		AvailableModels: models,
	})
	c.revision++
	c.mu.Unlock()
	c.notify()
	return true
}

func decodeAvailableModels(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var stringsList []string
	if err := json.Unmarshal(raw, &stringsList); err == nil && len(stringsList) > 0 {
		return cleanModelList(stringsList)
	}
	var objectsList []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &objectsList); err == nil {
		out := make([]string, 0, len(objectsList))
		for _, item := range objectsList {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				id = strings.TrimSpace(item.Value)
			}
			if id != "" {
				out = append(out, id)
			}
		}
		return cleanModelList(out)
	}
	return nil
}

func cleanModelList(models []string) []string {
	out := make([]string, 0, len(models))
	seen := make(map[string]bool, len(models))
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func (c *controller) applySessionSkills(client *acpclient.Client, result sessionSkillsResult) bool {
	sessionID := strings.TrimSpace(result.SessionID)
	if client == nil || sessionID == "" {
		return false
	}
	c.mu.Lock()
	session, ok := desktopSessionByID(c.state, sessionID)
	if !ok || sessionID != c.state.ActiveSessionID || !c.clientCurrentLocked(session.AgentID, client) {
		c.mu.Unlock()
		return false
	}
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventSessionSkillsUpdated,
		SessionID: sessionID,
		Skills:    projectSessionSkills(result),
	})
	c.revision++
	c.mu.Unlock()
	c.notify()
	return true
}

func projectSessionSkills(result sessionSkillsResult) []desktopstate.SkillState {
	skills := make([]desktopstate.SkillState, 0, len(result.Skills))
	for _, item := range result.Skills {
		skills = append(skills, desktopstate.SkillState{
			Name:        strings.TrimSpace(item.Name),
			Description: strings.TrimSpace(item.Description),
			Scope:       strings.TrimSpace(item.Scope),
			Active:      item.Active,
			Locked:      item.Locked,
			LockStatus:  strings.TrimSpace(item.LockStatus),
			Resources:   item.Resources,
		})
	}
	return skills
}

func (c *controller) pruneSessionRefreshersLocked() {
	live := make(map[string]struct{}, len(c.state.Sessions))
	for _, session := range c.state.Sessions {
		live[session.ID] = struct{}{}
	}
	c.contextRefresh.prune(live)
	c.memoryRefresh.prune(live)
	c.runtimeRefresh.prune(live)
	c.skillsRefresh.prune(live)
}

func terminalToolStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "failed", "canceled", "cancelled", "interrupted":
		return true
	default:
		return false
	}
}
