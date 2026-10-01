//go:build desktop || desktop_gio

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	runtimecomponent "github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/runtime"
	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	ProtonmanAgentID        = "protonman"
	acpAgentsEnvironment    = "PROTONMAN_ACP_AGENTS_JSON"
	reconnectInitialDelay   = time.Second
	reconnectMaxDelay       = 8 * time.Second
	reconnectRequestTimeout = 15 * time.Second
	newSessionTimeout       = 30 * time.Second
)

type ConnectionPhase string

const (
	ConnectionConnecting   ConnectionPhase = "connecting"
	ConnectionConnected    ConnectionPhase = "connected"
	ConnectionReconnecting ConnectionPhase = "reconnecting"
)

type acpSession struct {
	ID                    string   `json:"sessionId"`
	AgentID               string   `json:"agentId"`
	Title                 string   `json:"title"`
	UpdatedAt             string   `json:"updatedAt"`
	Cwd                   string   `json:"cwd"`
	WorkspaceKey          string   `json:"workspaceKey"`
	WorkspaceName         string   `json:"workspaceName"`
	AdditionalDirectories []string `json:"additionalDirectories"`
}

type Snapshot struct {
	State                 desktopstate.State
	Connection            ConnectionPhase
	Status                string
	ActiveAgentID         string
	AgentProfiles         []app.ACPAgentProfile
	AgentConnections      map[string]ConnectionPhase
	AgentStatuses         map[string]string
	AgentConfigOverridden bool
	AgentUpdating         bool
	AgentError            string
	CreatingSession       bool
	HistoryState          HistoryState
	RuntimeUpdating       bool
	MCPUpdating           bool
	MCPReconnecting       bool
	MCPError              string
	Revision              uint64
	PinnedSessions        []string
	CustomTitles          map[string]string
	FilterMode            string
	Theme                 string
	AgentDefaultModels    map[string]string
	AgentAvailableModels  map[string][]string
	Providers             []desktopstate.ProviderState
	ActiveProvider        string
	ActiveModel           string
	ProviderModels        map[string][]string
	ProviderModelsLoading map[string]bool
	ProviderUpdating      bool
	ProviderError         string
}

type SnapshotCache struct {
	valid    bool
	revision uint64
	value    Snapshot
}

type controller struct {
	ctx      context.Context
	cancel   context.CancelFunc
	onChange func()

	sessionLocks  agentSessionLockRegistry
	mu            sync.RWMutex
	state         desktopstate.State
	profiles      map[string]app.ACPAgentProfile
	clients       map[string]*acpclient.Client
	agentFeatures map[string]acpAgentFeatures
	connections   map[string]ConnectionPhase
	statuses      map[string]string
	activeAgentID string

	preferences           *app.DesktopPreferences
	pinnedSessions        []string
	customTitles          map[string]string
	agentDefaultModels    map[string]string
	agentAvailableModels  map[string][]string
	activeProvider        string
	activeModel           string
	providerModelsCache   map[string][]string
	providerModelsLoading map[string]bool
	providerUpdating      bool
	providerError         string
	filterMode            string
	theme                 string

	histories               map[string]HistoryState
	historyLoads            map[string]*sessionHistoryLoad
	historyStaging          map[string][]desktopstate.Event
	historyStagingBytes     map[string]int
	historyStagingTruncated map[string]bool
	timelineBytes           map[string]int
	messageStreams          map[messageStreamKey]string
	messageStreamBuffers    map[messageStreamKey]*messageStreamBuffer
	messageSequence         map[string]uint64
	permissionWait          map[string]chan string
	questionWait            map[string]chan desktopstate.QuestionResponse
	contextRefresh          *sessionRefreshTracker
	memoryRefresh           *sessionRefreshTracker
	runtimeRefresh          *sessionRefreshTracker
	skillsRefresh           *sessionRefreshTracker
	runtimeMutation         string

	agentProfiles         app.ACPAgents
	agentMutation         bool
	agentConfigOverridden bool
	agentError            string

	mcpIntegrations app.MCPIntegrations
	mcpMutation     bool
	mcpReconnect    bool
	mcpError        string

	creatingSession bool
	revision        uint64
	snapshotCache   SnapshotCache
}

func New(parent context.Context, onChange func(), agents app.ACPAgents, integrations app.MCPIntegrations, preferences *app.DesktopPreferences) *controller {
	ctx, cancel := context.WithCancel(parent)
	defaults := []app.ACPAgentProfile{DefaultACPAgentProfile()}
	profiles, profileErr := agents.Resolve(ctx, defaults, os.Getenv(acpAgentsEnvironment))
	agentError := ""
	if profileErr != nil {
		profiles = defaults
		agentError = compactError(profileErr)
	}
	instance := &controller{
		ctx:                     ctx,
		cancel:                  cancel,
		onChange:                onChange,
		profiles:                make(map[string]app.ACPAgentProfile, len(profiles)),
		clients:                 make(map[string]*acpclient.Client, len(profiles)),
		agentFeatures:           make(map[string]acpAgentFeatures, len(profiles)),
		connections:             make(map[string]ConnectionPhase, len(profiles)),
		statuses:                make(map[string]string, len(profiles)),
		histories:               make(map[string]HistoryState),
		historyLoads:            make(map[string]*sessionHistoryLoad),
		historyStaging:          make(map[string][]desktopstate.Event),
		historyStagingBytes:     make(map[string]int),
		historyStagingTruncated: make(map[string]bool),
		timelineBytes:           make(map[string]int),
		messageStreams:          make(map[messageStreamKey]string),
		messageStreamBuffers:    make(map[messageStreamKey]*messageStreamBuffer),
		messageSequence:         make(map[string]uint64),
		permissionWait:          make(map[string]chan string),
		contextRefresh:          newSessionRefreshTracker(contextRefreshInterval),
		memoryRefresh:           newSessionRefreshTracker(memoryRefreshInterval),
		runtimeRefresh:          newSessionRefreshTracker(runtimeRefreshInterval),
		skillsRefresh:           newSessionRefreshTracker(skillsRefreshInterval),
		agentProfiles:           agents,
		agentConfigOverridden:   strings.TrimSpace(os.Getenv(acpAgentsEnvironment)) != "",
		agentError:              agentError,
		mcpIntegrations:         integrations,
		preferences:             preferences,
		pinnedSessions:          make([]string, 0),
		customTitles:            make(map[string]string),
		agentDefaultModels:      make(map[string]string),
		agentAvailableModels:    make(map[string][]string),
		providerModelsCache:     make(map[string][]string),
		providerModelsLoading:   make(map[string]bool),
		filterMode:              "all",
		theme:                   "system",
	}
	for _, profile := range profiles {
		instance.profiles[profile.ID] = CloneACPAgentProfile(profile)
		instance.connections[profile.ID] = ConnectionConnecting
		instance.statuses[profile.ID] = "Starting " + profile.DisplayName + "…"
	}
	instance.activeAgentID = preferredAgentID(profiles)
	if integrations.Available() {
		instance.loadMCPIntegrations()
	}
	if preferences != nil && preferences.Available() {
		instance.loadPreferences()
	}
	go instance.run()
	return instance
}

func (c *controller) loadPreferences() {
	state, err := c.preferences.Load(c.ctx)
	if err != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pinnedSessions = slices.Clone(state.PinnedSessions)
	c.customTitles = cloneCustomTitles(state.CustomTitles)
	if len(state.AgentDefaultModels) > 0 {
		c.agentDefaultModels = make(map[string]string, len(state.AgentDefaultModels))
		for k, v := range state.AgentDefaultModels {
			c.agentDefaultModels[k] = v
		}
	}
	if len(state.AgentAvailableModels) > 0 {
		c.agentAvailableModels = make(map[string][]string, len(state.AgentAvailableModels))
		for k, v := range state.AgentAvailableModels {
			c.agentAvailableModels[k] = slices.Clone(v)
		}
	}
	if state.FilterMode != "" {
		c.filterMode = state.FilterMode
	}
	if state.Theme != "" {
		c.theme = state.Theme
	}
	c.revision++
}

func (c *controller) SetTheme(theme string) {
	c.mu.Lock()
	c.theme = theme
	c.revision++
	c.mu.Unlock()
	c.notify()

	if c.preferences != nil {
		go func() {
			_ = c.preferences.SetTheme(c.ctx, theme)
		}()
	}
}

func cloneCustomTitles(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (c *controller) Close() {
	c.cancel()
	c.mu.Lock()
	clients := make([]*acpclient.Client, 0, len(c.clients))
	for _, client := range c.clients {
		clients = append(clients, client)
	}
	clear(c.clients)
	for sessionID, load := range c.historyLoads {
		load.cancel()
		delete(c.historyLoads, sessionID)
	}
	for _, buffer := range c.messageStreamBuffers {
		if buffer.timer != nil {
			buffer.timer.Stop()
		}
	}
	clear(c.messageStreamBuffers)
	clear(c.messageStreams)
	c.mu.Unlock()
	for _, client := range clients {
		_ = client.Close()
	}
}

func (c *controller) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snapshotCache.valid && c.snapshotCache.revision == c.revision {
		return c.snapshotCache.value
	}
	snapshot := Snapshot{
		State:                 desktopstate.ClonePresentationState(c.state),
		Connection:            c.connections[c.activeAgentID],
		Status:                c.statuses[c.activeAgentID],
		ActiveAgentID:         c.activeAgentID,
		AgentProfiles:         CloneACPAgentProfiles(c.profiles),
		AgentConnections:      make(map[string]ConnectionPhase, len(c.connections)),
		AgentStatuses:         make(map[string]string, len(c.statuses)),
		AgentConfigOverridden: c.agentConfigOverridden,
		AgentUpdating:         c.agentMutation,
		AgentError:            c.agentError,
		CreatingSession:       c.creatingSession,
		HistoryState:          HistoryStateUnloaded,
		RuntimeUpdating:       c.runtimeMutation != "",
		MCPUpdating:           c.mcpMutation,
		MCPReconnecting:       c.mcpReconnect,
		MCPError:              c.mcpError,
		Revision:              c.revision,
		PinnedSessions:        slices.Clone(c.pinnedSessions),
		CustomTitles:          cloneCustomTitles(c.customTitles),
		FilterMode:            c.filterMode,
		Theme:                 c.theme,
		AgentDefaultModels:    make(map[string]string, len(c.agentDefaultModels)),
		AgentAvailableModels:  make(map[string][]string, len(c.agentAvailableModels)),
		Providers:             slices.Clone(c.state.Providers),
		ActiveProvider:        c.activeProvider,
		ActiveModel:           c.activeModel,
		ProviderModels:        cloneProviderModels(c.providerModelsCache),
		ProviderModelsLoading: cloneBoolMap(c.providerModelsLoading),
		ProviderUpdating:      c.providerUpdating,
		ProviderError:         c.providerError,
	}
	for agentID, phase := range c.connections {
		snapshot.AgentConnections[agentID] = phase
	}
	for agentID, status := range c.statuses {
		snapshot.AgentStatuses[agentID] = status
	}
	for agentID, model := range c.agentDefaultModels {
		snapshot.AgentDefaultModels[agentID] = model
	}
	for agentID, models := range c.agentAvailableModels {
		snapshot.AgentAvailableModels[agentID] = slices.Clone(models)
	}
	activeKey := desktopSessionStorageKey(snapshot.State, desktopstate.SessionRef{AgentID: snapshot.State.ActiveAgentID, SessionID: snapshot.State.ActiveSessionID})
	if state, ok := c.histories[activeKey]; ok {
		snapshot.HistoryState = state
	}
	c.snapshotCache.valid = true
	c.snapshotCache.revision = c.revision
	c.snapshotCache.value = snapshot
	return snapshot
}

func (c *controller) setAgentAvailableModelsLocked(agentID string, models []string) {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	if agentID == "" {
		agentID = ProtonmanAgentID
	}
	if len(models) == 0 {
		return
	}
	if c.agentAvailableModels == nil {
		c.agentAvailableModels = make(map[string][]string)
	}
	c.agentAvailableModels[agentID] = slices.Clone(models)
	if c.preferences != nil {
		cloned := slices.Clone(models)
		go func(aID string, m []string) {
			_ = c.preferences.SetAgentAvailableModels(c.ctx, aID, m)
		}(agentID, cloned)
	}
}

// advanceSnapshotCacheForTimelineLocked reuses the cached presentation snapshot
// when a stream flush changed only one session's timeline. It path-copies the
// sessions slice and the active timeline, keeping snapshots already returned to
// the UI immutable while avoiding clones of unrelated state.
func (c *controller) advanceSnapshotCacheForTimelineLocked(sessionID string, agentIDs ...string) bool {
	cache := &c.snapshotCache
	agentID := c.state.ActiveAgentID
	if len(agentIDs) > 0 && agentIDs[0] != "" {
		agentID = agentIDs[0]
	}
	if !cache.valid || cache.revision+1 != c.revision || cache.value.State.ActiveSessionID != sessionID || cache.value.State.ActiveAgentID != agentID {
		return false
	}
	index, ok := sessionIndex(cache.value.State.Sessions, sessionID, agentID)
	if !ok {
		return false
	}
	live := desktopstateSessionPointer(&c.state, sessionID, agentID)
	if live == nil {
		return false
	}

	next := cache.value
	next.State.Sessions = slices.Clone(cache.value.State.Sessions)
	nextSession := next.State.Sessions[index]
	nextSession.Timeline = slices.Clone(live.Timeline)
	nextSession.HistoryTruncated = live.HistoryTruncated
	next.State.Sessions[index] = nextSession
	next.Revision = c.revision
	cache.value = next
	cache.revision = c.revision
	return true
}

func (c *controller) selectSession(sessionID string) {
	c.SelectSessionForAgent("", sessionID)
}

func (c *controller) SelectSessionForAgent(agentID, sessionID string) {
	c.mu.Lock()
	previousSessionID := c.state.ActiveSessionID
	previousAgentID := c.state.ActiveAgentID
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventSessionSelected,
		AgentID:   agentID,
		SessionID: sessionID,
	})
	if session, ok := SessionByID(c.state, c.state.ActiveSessionID, c.state.ActiveAgentID); ok {
		c.state.ActiveProjectID = session.ProjectID
		if strings.TrimSpace(session.AgentID) != "" {
			c.activeAgentID = session.AgentID
		} else {
			c.activeAgentID = ProtonmanAgentID
		}
	}
	c.pruneInactiveSessionHistoryLocked(c.state.ActiveSessionID, previousSessionID,
		desktopstate.SessionRef{AgentID: c.state.ActiveAgentID, SessionID: c.state.ActiveSessionID},
		desktopstate.SessionRef{AgentID: previousAgentID, SessionID: previousSessionID},
	)
	c.revision++
	c.snapshotCache = SnapshotCache{}
	activeSessionID := c.state.ActiveSessionID
	c.mu.Unlock()
	c.notify()
	c.loadSessionHistory(activeSessionID)
	c.RefreshActiveSession(true)
}

func (c *controller) DeleteSession(sessionID string, agentIDs ...string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	var (
		targetAgentID   string
		targetProjectID string
		storageKey      string
	)
	if len(agentIDs) > 0 {
		targetAgentID = agentIDs[0]
	}
	for i := range c.state.Sessions {
		if c.state.Sessions[i].ID == sessionID && (targetAgentID == "" || c.state.Sessions[i].AgentID == targetAgentID) {
			targetAgentID = c.state.Sessions[i].AgentID
			targetProjectID = c.state.Sessions[i].ProjectID
			storageKey = desktopSessionStorageKey(c.state, c.state.Sessions[i].Ref())
			break
		}
	}
	if storageKey == "" {
		c.mu.Unlock()
		return
	}
	client := c.clients[targetAgentID]
	activeSessionDeleted := c.state.ActiveSessionID == sessionID && (c.state.ActiveAgentID == "" || c.state.ActiveAgentID == targetAgentID)
	c.mu.Unlock()

	if client != nil {
		type sessionDeleteParams struct {
			SessionID string `json:"sessionId"`
		}
		_ = client.Call(c.ctx, "session/delete", sessionDeleteParams{SessionID: sessionID}, nil)
	}

	if c.preferences != nil {
		_ = c.preferences.RemoveSession(c.ctx, storageKey)
	}

	c.mu.Lock()
	newSessions := make([]desktopstate.SessionState, 0, len(c.state.Sessions))
	var fallbackSessionID, fallbackAgentID string
	for _, sess := range c.state.Sessions {
		if sess.ID != sessionID || sess.AgentID != targetAgentID {
			newSessions = append(newSessions, sess)
			if activeSessionDeleted && sess.AgentID == targetAgentID && sess.ProjectID == targetProjectID && fallbackSessionID == "" {
				fallbackSessionID = sess.ID
				fallbackAgentID = sess.AgentID
			}
		}
	}
	c.state.Sessions = newSessions

	if activeSessionDeleted {
		c.state.ActiveSessionID = fallbackSessionID
		c.state.ActiveAgentID = fallbackAgentID
	}

	if load, ok := c.historyLoads[storageKey]; ok {
		load.cancel()
		delete(c.historyLoads, storageKey)
	}
	delete(c.histories, storageKey)
	delete(c.historyStaging, storageKey)
	delete(c.historyStagingBytes, storageKey)
	delete(c.historyStagingTruncated, storageKey)
	delete(c.timelineBytes, storageKey)

	if c.preferences != nil {
		snap := c.preferences.Snapshot()
		c.pinnedSessions = slices.Clone(snap.PinnedSessions)
		c.customTitles = cloneCustomTitles(snap.CustomTitles)
		c.filterMode = snap.FilterMode
	}

	c.revision++
	c.mu.Unlock()

	if activeSessionDeleted && fallbackSessionID != "" {
		c.SelectSessionForAgent(fallbackAgentID, fallbackSessionID)
	} else {
		c.notify()
	}
}

func (c *controller) RenameSession(sessionID, newTitle string, agentIDs ...string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	var storageKey string
	for _, session := range c.state.Sessions {
		if session.ID == sessionID && (len(agentIDs) == 0 || agentIDs[0] == "" || session.AgentID == agentIDs[0]) {
			storageKey = desktopSessionStorageKey(c.state, session.Ref())
			break
		}
	}
	c.mu.Unlock()
	if storageKey == "" {
		return
	}
	newTitle = strings.TrimSpace(newTitle)
	if c.preferences != nil {
		_ = c.preferences.SetCustomTitle(c.ctx, storageKey, newTitle)
	}
	c.mu.Lock()
	if c.preferences != nil {
		c.customTitles = cloneCustomTitles(c.preferences.Snapshot().CustomTitles)
	} else {
		if c.customTitles == nil {
			c.customTitles = make(map[string]string)
		}
		if newTitle == "" {
			delete(c.customTitles, storageKey)
		} else {
			c.customTitles[storageKey] = newTitle
		}
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) ToggleSkill(sessionID string, skillName string) {
	sessionID = strings.TrimSpace(sessionID)
	skillName = strings.TrimSpace(skillName)
	if sessionID == "" || skillName == "" {
		return
	}
	c.mu.Lock()
	client, agentID := c.clientForSessionLocked(sessionID, c.state.ActiveAgentID)
	if client == nil || c.connections[agentID] != ConnectionConnected {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	go func() {
		var result struct {
			SessionID string `json:"sessionId"`
			Name      string `json:"name"`
			Active    bool   `json:"active"`
		}
		if err := c.callInspector(client, "protonman/session/skills/toggle", map[string]any{
			"sessionId": sessionID,
			"name":      skillName,
		}, &result); err != nil {
			return
		}
		c.refreshSessionSkills(sessionID, true, agentID)
	}()
}

func (c *controller) TogglePinSession(sessionID string, agentIDs ...string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	var storageKey string
	for _, session := range c.state.Sessions {
		if session.ID == sessionID && (len(agentIDs) == 0 || agentIDs[0] == "" || session.AgentID == agentIDs[0]) {
			storageKey = desktopSessionStorageKey(c.state, session.Ref())
			break
		}
	}
	c.mu.Unlock()
	if storageKey == "" {
		return
	}
	if c.preferences != nil {
		_, _ = c.preferences.TogglePin(c.ctx, storageKey)
	}
	c.mu.Lock()
	if c.preferences != nil {
		c.pinnedSessions = slices.Clone(c.preferences.Snapshot().PinnedSessions)
	} else {
		idx := slices.Index(c.pinnedSessions, storageKey)
		if idx >= 0 {
			c.pinnedSessions = slices.Delete(c.pinnedSessions, idx, idx+1)
		} else {
			c.pinnedSessions = append(c.pinnedSessions, storageKey)
		}
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) SetFilterMode(mode string) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "running" && mode != "pinned" {
		mode = "all"
	}
	if c.preferences != nil {
		_ = c.preferences.SetFilterMode(c.ctx, mode)
	}
	c.mu.Lock()
	c.filterMode = mode
	c.revision++
	c.mu.Unlock()
	c.notify()
}

type acpConfigOption struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CurrentValue any    `json:"currentValue"`
	Options      []struct {
		Value string `json:"value"`
		Name  string `json:"name"`
	} `json:"options"`
}

func extractModelsFromConfigOptions(options []acpConfigOption) (models []string, currentModel string) {
	for _, opt := range options {
		if strings.TrimSpace(opt.ID) == "model" {
			currentModel = acpConfigStringValue(opt.CurrentValue)
			for _, o := range opt.Options {
				val := strings.TrimSpace(o.Value)
				if val != "" {
					models = append(models, val)
				}
			}
			return models, currentModel
		}
	}
	return nil, ""
}

type acpModelsResult struct {
	CurrentModelID  string `json:"currentModelId"`
	AvailableModels []struct {
		ModelID string `json:"modelId"`
		Name    string `json:"name"`
	} `json:"availableModels"`
}

func extractModelsFromACP(options []acpConfigOption, modelsResult *acpModelsResult) ([]string, string) {
	if modelsResult != nil && len(modelsResult.AvailableModels) > 0 {
		var models []string
		for _, m := range modelsResult.AvailableModels {
			id := strings.TrimSpace(m.ModelID)
			if id != "" {
				models = append(models, id)
			}
		}
		return models, strings.TrimSpace(modelsResult.CurrentModelID)
	}
	return extractModelsFromConfigOptions(options)
}

func (c *controller) NewSession() {
	c.mu.Lock()
	agentID := c.activeAgentID
	if agentID == "" {
		agentID = c.agentForProjectLocked(c.state.ActiveProjectID)
	}
	client := c.clients[agentID]
	if c.connections[agentID] != ConnectionConnected || client == nil || c.creatingSession {
		c.mu.Unlock()
		return
	}
	workspace, err := newSessionWorkspace(c.state)
	if err != nil {
		c.statuses[agentID] = "New session unavailable · " + compactError(err)
		c.revision++
		c.mu.Unlock()
		c.notify()
		return
	}
	c.activeAgentID = agentID
	c.creatingSession = true
	c.statuses[agentID] = "Creating session…"
	c.revision++
	requestedAdditionalDirectories := c.projectAdditionalDirectoriesLocked(c.state.ActiveProjectID, workspace)
	features := c.agentFeatures[agentID]
	profile := c.profiles[agentID]
	additionalDirectories := requestedAdditionalDirectories
	if !features.SupportsAdditionalDirectories {
		additionalDirectories = nil
	}
	c.mu.Unlock()
	c.notify()
	params := c.mcpNewSessionParams(workspace, additionalDirectories)

	go func() {
		defer func() {
			c.mu.Lock()
			c.creatingSession = false
			c.revision++
			c.mu.Unlock()
			c.notify()
		}()

		defer c.lockAgentSession(agentID)()
		var result struct {
			SessionID string `json:"sessionId"`
			Modes     *struct {
				CurrentModeID string `json:"currentModeId"`
			} `json:"modes,omitempty"`
			ConfigOptions []acpConfigOption `json:"configOptions,omitempty"`
			Models        *acpModelsResult  `json:"models,omitempty"`
		}
		err := createACPSession(
			c.ctx,
			client,
			profile,
			features.AuthMethods,
			params,
			&result,
			func(methodName string) {
				methodName = strings.TrimSpace(methodName)
				if methodName == "" {
					methodName = "agent"
				}
				if strings.HasPrefix(strings.ToLower(methodName), "sign in with ") {
					c.setAgentStatus(agentID, methodName+"…")
					return
				}
				c.setAgentStatus(agentID, "Signing in with "+methodName+"…")
			},
		)
		if err == nil && strings.TrimSpace(result.SessionID) == "" {
			err = errors.New("ACP returned an empty session ID")
		}

		c.mu.Lock()
		if c.clients[agentID] != client {
			c.mu.Unlock()
			return
		}
		if err != nil {
			c.statuses[agentID] = "New session failed · " + compactError(err)
			c.mu.Unlock()
			return
		}
		c.state = addLocalSession(c.state, result.SessionID, workspace, agentID)
		if models, currentModel := extractModelsFromACP(result.ConfigOptions, result.Models); len(models) > 0 {
			c.setAgentAvailableModelsLocked(agentID, models)
			if sess := desktopstateSessionPointer(&c.state, result.SessionID, agentID); sess != nil {
				sess.AvailableModels = models
				if currentModel != "" && sess.Runtime.Model == "" {
					sess.Runtime.Model = currentModel
				} else if sess.Runtime.Model == "" && c.agentDefaultModels[agentID] != "" {
					sess.Runtime.Model = c.agentDefaultModels[agentID]
				}
			}
		} else if c.agentAvailableModels != nil && len(c.agentAvailableModels[agentID]) > 0 {
			if sess := desktopstateSessionPointer(&c.state, result.SessionID, agentID); sess != nil {
				sess.AvailableModels = slices.Clone(c.agentAvailableModels[agentID])
				if sess.Runtime.Model == "" && c.agentDefaultModels[agentID] != "" {
					sess.Runtime.Model = c.agentDefaultModels[agentID]
				}
			}
		}
		if result.Modes != nil && strings.TrimSpace(result.Modes.CurrentModeID) != "" {
			if sess := desktopstateSessionPointer(&c.state, result.SessionID, agentID); sess != nil {
				modeID := strings.TrimSpace(result.Modes.CurrentModeID)
				if isClineACPProfile(profile) {
					sess.Runtime.PermissionMode = clinePermissionMode(modeID, clineAutoApproveValue(result.ConfigOptions, false))
				} else {
					sess.Runtime.PermissionMode = modeID
				}
			}
		}
		c.state.ActiveSessionID = result.SessionID
		c.state.ActiveAgentID = agentID
		c.state.ActiveProjectID = workspaceKey(workspace, result.SessionID)
		storageKey := desktopSessionStorageKey(c.state, desktopstate.SessionRef{AgentID: agentID, SessionID: result.SessionID})
		c.histories[storageKey] = HistoryStateLoaded
		c.clearMessageStreamsLocked(result.SessionID, agentID)
		c.setProjectDefaultAgentLocked(c.state.ActiveProjectID, agentID)
		c.statuses[agentID] = "Session created"
		if len(requestedAdditionalDirectories) > 0 && !features.SupportsAdditionalDirectories {
			c.statuses[agentID] = "Session created · agent uses the primary folder only"
		}
		c.revision++
		c.mu.Unlock()
		c.RefreshActiveSession(true)
	}()
}

func (c *controller) run() {
	profiles := c.sortedProfiles()
	for _, profile := range profiles {
		go c.superviseAgent(profile)
	}
	<-c.ctx.Done()
}

func (c *controller) superviseAgent(profile app.ACPAgentProfile) {
	delay := reconnectInitialDelay
	for {
		if c.ctx.Err() != nil {
			return
		}
		c.setAgentConnection(profile.ID, ConnectionConnecting, "Starting "+profile.DisplayName+"…")
		var startedClient atomic.Pointer[acpclient.Client]
		client, err := acpclient.StartCommand(c.ctx, CommandSpecForACPAgent(profile), func(event acpclient.Event) {
			if current := startedClient.Load(); current != nil {
				c.handleACPEventForAgent(profile.ID, current, event)
			}
		})
		if err != nil {
			c.setAgentConnection(profile.ID, ConnectionReconnecting, "Start failed · "+compactError(err))
			if !waitForReconnect(c.ctx, delay) {
				return
			}
			delay = nextReconnectDelay(delay)
			continue
		}
		startedClient.Store(client)

		features, err := initializeACP(c.ctx, client)
		if err != nil {
			fmt.Printf("[superviseAgent %s] initializeACP failed: %v\n", profile.ID, err)
			_ = client.Close()
			if c.ctx.Err() != nil {
				return
			}
			c.setAgentConnection(profile.ID, ConnectionReconnecting, "Connection failed · "+compactError(err))
			if !waitForReconnect(c.ctx, delay) {
				return
			}
			delay = nextReconnectDelay(delay)
			continue
		}
		c.setAgentFeatures(profile.ID, features)
		client.SetRequestHandler(func(requestCtx context.Context, request acpclient.Request) (any, error) {
			switch request.Method {
			case requestPermissionMethod:
				return c.handlePermissionRequestForAgent(profile.ID, client, requestCtx, request)
			case requestQuestionMethod:
				return c.handleQuestionRequestForAgent(profile.ID, client, requestCtx, request)
			default:
				return nil, fmt.Errorf("%w: %s", acpclient.ErrMethodNotHandled, request.Method)
			}
		})

		c.setClient(profile.ID, client)
		delay = reconnectInitialDelay
		if err := c.refreshSessions(profile.ID, client); err != nil {
			if isACPMethodNotFound(err) {
				c.setAgentStatus(profile.ID, "Connected · session list unavailable")
			} else {
				fmt.Printf("[superviseAgent %s] refreshSessions failed: %v\n", profile.ID, err)
				c.setAgentStatus(profile.ID, "Connected · session refresh failed")
			}
		}
		c.resumeKnownSessions(profile.ID, client)

		select {
		case <-c.ctx.Done():
			_ = client.Close()
			return
		case <-client.Done():
		}

		fmt.Printf("[superviseAgent %s] client done\n", profile.ID)
		c.markAgentDisconnected(profile.ID, client)
		if c.ctx.Err() != nil {
			return
		}
		if !waitForReconnect(c.ctx, delay) {
			return
		}
	}
}

func (c *controller) refreshSessions(agentID string, client *acpclient.Client) error {
	if !c.agentSupportsSessionList(agentID) {
		c.setAgentStatus(agentID, "Connected · session list unavailable")
		return nil
	}
	defer c.lockAgentSession(agentID)()
	callCtx, cancel := context.WithTimeout(c.ctx, reconnectRequestTimeout)
	defer cancel()
	var result struct {
		Sessions []acpSession `json:"sessions"`
	}
	if err := client.Call(callCtx, "session/list", map[string]any{}, &result); err != nil {
		return err
	}
	c.mu.Lock()
	if c.clients[agentID] != client {
		c.mu.Unlock()
		return errSessionHistoryClientChanged
	}
	next := projectSessions(c.state, agentID, result.Sessions)
	c.state = next
	c.pruneSessionRuntimeLocked()
	c.pruneSessionRefreshersLocked()
	c.revision++
	activeSessionID := c.state.ActiveSessionID
	c.mu.Unlock()
	c.notify()
	c.loadSessionHistory(activeSessionID)
	c.RefreshActiveSession(true)
	return nil
}

func (c *controller) setAgentConnection(agentID string, phase ConnectionPhase, status string) {
	c.mu.Lock()
	c.connections[agentID] = phase
	c.statuses[agentID] = status
	if agentID == c.activeAgentID {
		c.mcpReconnect = c.mcpReconnect && phase != ConnectionConnected
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) setStatus(status string) {
	c.mu.Lock()
	if c.statuses == nil {
		c.statuses = make(map[string]string)
	}
	c.statuses[c.activeAgentID] = status
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) setAgentStatus(agentID, status string) {
	c.mu.Lock()
	if c.statuses == nil {
		c.statuses = make(map[string]string)
	}
	c.statuses[agentID] = status
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) setClient(agentID string, client *acpclient.Client) {
	c.mu.Lock()
	c.clients[agentID] = client
	c.connections[agentID] = ConnectionConnected
	status := "Connected"
	if c.mcpError != "" {
		status = "Connected · MCP settings unavailable"
	}
	c.statuses[agentID] = status
	if len(c.clients) >= len(c.profiles) {
		c.mcpReconnect = false
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
	if runtimecomponent.IsProtonmanAgent(agentID) {
		go c.RefreshProviders()
	}
}

func (c *controller) markAgentDisconnected(agentID string, client *acpclient.Client) {
	c.mu.Lock()
	if c.clients[agentID] != client {
		c.mu.Unlock()
		return
	}
	delete(c.clients, agentID)
	delete(c.agentFeatures, agentID)
	c.connections[agentID] = ConnectionReconnecting
	c.statuses[agentID] = "Disconnected · retrying"
	c.resetAgentTransientSessionStateLocked(agentID)
	c.state = desktopstate.MarkAgentDisconnected(c.state, agentID)
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) notify() {
	if c.onChange != nil {
		c.onChange()
	}
}

func initializeACP(ctx context.Context, client *acpclient.Client) (acpAgentFeatures, error) {
	callCtx, cancel := context.WithTimeout(ctx, reconnectRequestTimeout)
	defer cancel()
	var result initializeACPResponse
	err := client.Call(callCtx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo": map[string]any{
			"name":    "protonman-desktop-gio",
			"title":   "Protonman Desktop",
			"version": buildinfo.Version(),
		},
		"clientCapabilities": map[string]any{},
	}, &result)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return acpAgentFeatures{}, errors.New("connection closed unexpectedly (EOF)")
		}
		return acpAgentFeatures{}, err
	}
	if result.ProtocolVersion != 1 {
		return acpAgentFeatures{}, fmt.Errorf("unsupported ACP v%d", result.ProtocolVersion)
	}
	return result.features(), nil
}

func projectSessions(current desktopstate.State, agentID string, remoteSessions []acpSession) desktopstate.State {
	previous := make(map[string]desktopstate.SessionState, len(current.Sessions))
	for _, session := range current.Sessions {
		if session.AgentID == agentID || session.AgentID == "" && agentID == ProtonmanAgentID {
			previous[session.ID] = session
		}
	}
	sessions := make([]desktopstate.SessionState, 0, len(remoteSessions)+len(current.Sessions))
	for _, session := range current.Sessions {
		if session.AgentID != agentID && !(session.AgentID == "" && agentID == ProtonmanAgentID) {
			sessions = append(sessions, session)
		}
	}
	for _, remote := range remoteSessions {
		if strings.TrimSpace(remote.ID) == "" {
			continue
		}
		session := previous[remote.ID]
		session.ID = remote.ID
		session.AgentID = agentID
		if updatedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(remote.UpdatedAt)); err == nil && updatedAt.After(session.LastActivityAt) {
			session.LastActivityAt = updatedAt
		}
		previousTitle := session.Title
		session.Title = strings.TrimSpace(remote.Title)
		if session.Title == "" {
			session.Title = previousTitle
		}
		if session.Title == "" {
			session.Title = "Session " + shortID(remote.ID)
		}
		previousWorkspace := session.Workspace
		session.Workspace = strings.TrimSpace(remote.Cwd)
		if session.Workspace == "" {
			session.Workspace = previousWorkspace
		}
		if session.Workspace == "" {
			if proj, ok := projectByID(current, remote.WorkspaceKey); ok && len(proj.Folders) > 0 {
				session.Workspace = proj.Folders[0].Path
			}
		}
		if session.Workspace == "" {
			if defaultWS, err := newSessionWorkspace(current); err == nil && defaultWS != "" {
				wsKey := sessionWorkspaceKey(defaultWS)
				remoteKey := strings.TrimSpace(remote.WorkspaceKey)
				if remoteKey == "" {
					if parts := strings.Split(remote.ID, "-"); len(parts) >= 2 && len(parts[1]) == 16 && isHexString(parts[1]) {
						remoteKey = parts[1]
					}
				}
				if remoteKey == "" || remoteKey == wsKey || remoteKey == "workspace:"+canonicalWorkspacePath(defaultWS) {
					session.Workspace = defaultWS
				}
			}
		}
		if len(remote.AdditionalDirectories) > 0 {
			session.AdditionalDirectories = slices.Clone(remote.AdditionalDirectories)
		}
		previousWorkspaceKey := session.WorkspaceKey
		session.WorkspaceKey = strings.TrimSpace(remote.WorkspaceKey)
		if session.WorkspaceKey == "" {
			session.WorkspaceKey = previousWorkspaceKey
		}
		if session.WorkspaceKey == "" {
			session.WorkspaceKey = workspaceKey(session.Workspace, remote.ID)
		}
		previousWorkspaceName := session.WorkspaceName
		session.WorkspaceName = strings.TrimSpace(remote.WorkspaceName)
		if session.WorkspaceName == "" {
			session.WorkspaceName = previousWorkspaceName
		}
		if session.WorkspaceName == "" {
			session.WorkspaceName = workspaceName(session.Workspace, remote.ID)
		}
		session.ProjectID = session.WorkspaceKey
		if session.Status == "" {
			session.Status = desktopstate.TaskIdle
		}
		sessions = append(sessions, session)
	}

	next := desktopstate.Reduce(current, desktopstate.Event{
		Kind:     desktopstate.EventSessionsReplaced,
		Sessions: sessions,
	})
	next.Projects = deriveProjectsPreserving(current.Projects, sessions)
	if next.ActiveSessionID == "" && len(sessions) > 0 {
		next.ActiveSessionID = sessions[0].ID
		next.ActiveAgentID = sessions[0].AgentID
	}
	for _, session := range sessions {
		if session.ID == next.ActiveSessionID && (next.ActiveAgentID == "" || session.AgentID == next.ActiveAgentID) {
			next.ActiveProjectID = session.ProjectID
			break
		}
	}
	return next
}

func deriveProjects(sessions []desktopstate.SessionState) []desktopstate.ProjectState {
	return deriveProjectsPreserving(nil, sessions)
}

func deriveProjectsPreserving(previous []desktopstate.ProjectState, sessions []desktopstate.SessionState) []desktopstate.ProjectState {
	projects := make([]desktopstate.ProjectState, 0)
	projectIndex := make(map[string]int)
	for _, project := range previous {
		projectIndex[project.ID] = len(projects)
		projects = append(projects, project)
	}
	for _, session := range sessions {
		index, ok := projectIndex[session.ProjectID]
		if !ok {
			projects = append(projects, desktopstate.ProjectState{
				ID:             session.ProjectID,
				Name:           session.WorkspaceName,
				DefaultAgentID: session.AgentID,
			})
			index = len(projects) - 1
			projectIndex[session.ProjectID] = index
		}
		project := &projects[index]
		if project.Name == "" {
			project.Name = session.WorkspaceName
		}
		if session.Workspace != "" && !slices.ContainsFunc(project.Folders, func(folder desktopstate.ProjectFolder) bool {
			return folder.Path == session.Workspace
		}) {
			project.Folders = append(project.Folders, desktopstate.ProjectFolder{
				Path:    session.Workspace,
				Primary: len(project.Folders) == 0,
			})
		}
		if session.AgentID != "" && !slices.Contains(project.AgentIDs, session.AgentID) {
			project.AgentIDs = append(project.AgentIDs, session.AgentID)
		}
		if project.DefaultAgentID == "" {
			project.DefaultAgentID = session.AgentID
		}
	}
	liveProjects := projects[:0]
	for _, project := range projects {
		if len(project.Folders) > 0 || project.DefaultAgentID != "" || len(project.AgentIDs) > 0 {
			liveProjects = append(liveProjects, project)
		}
	}
	projects = liveProjects
	for index := range projects {
		slices.Sort(projects[index].AgentIDs)
	}
	slices.SortFunc(projects, func(left, right desktopstate.ProjectState) int {
		if compared := strings.Compare(left.Name, right.Name); compared != 0 {
			return compared
		}
		return strings.Compare(left.ID, right.ID)
	})
	return projects
}

func addLocalSession(state desktopstate.State, sessionID, workspace, agentID string) desktopstate.State {
	next := desktopstate.CloneState(state)
	for _, session := range next.Sessions {
		if session.ID == sessionID && session.AgentID == agentID {
			return next
		}
	}
	workspace = strings.TrimSpace(workspace)
	next.Sessions = append(next.Sessions, desktopstate.SessionState{
		ID:             sessionID,
		AgentID:        agentID,
		ProjectID:      workspaceKey(workspace, sessionID),
		Title:          "Session " + shortID(sessionID),
		Workspace:      workspace,
		WorkspaceKey:   workspaceKey(workspace, sessionID),
		WorkspaceName:  workspaceName(workspace, sessionID),
		LastActivityAt: time.Now().UTC(),
		Status:         desktopstate.TaskIdle,
	})
	next.Projects = deriveProjectsPreserving(next.Projects, next.Sessions)
	return next
}

func newSessionWorkspace(state desktopstate.State) (string, error) {
	if override := strings.TrimSpace(os.Getenv("PROTONMAN_GIO_WORKSPACE")); override != "" {
		return existingWorkspaceDirectory(override)
	}
	candidates := make([]string, 0, 1)
	for _, project := range state.Projects {
		if project.ID == state.ActiveProjectID {
			for _, folder := range project.Folders {
				if strings.TrimSpace(folder.Path) != "" {
					candidates = append(candidates, folder.Path)
				}
			}
			break
		}
	}
	if len(candidates) == 0 {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return "", err
		}
		candidates = append(candidates, workingDirectory)
	}
	for _, candidate := range candidates {
		if absolute, err := existingWorkspaceDirectory(candidate); err == nil {
			return absolute, nil
		}
	}
	return "", errors.New("choose an existing workspace directory")
}

// projectAdditionalDirectoriesLocked returns the active project's remaining
// folders, excluding the primary workspace and anything that no longer exists or
// resolves to the same directory. Callers must hold c.mu.
func (c *controller) projectAdditionalDirectoriesLocked(projectID, primary string) []string {
	primary = canonicalWorkspacePath(primary)
	directories := make([]string, 0, 2)
	for _, project := range c.state.Projects {
		if project.ID != projectID {
			continue
		}
		for _, folder := range project.Folders {
			candidate := strings.TrimSpace(folder.Path)
			if candidate == "" {
				continue
			}
			// Only directories that still exist are worth authorizing; a stale
			// folder would make the child fail the whole session registration.
			resolved, err := existingWorkspaceDirectory(candidate)
			if err != nil {
				continue
			}
			if resolved == primary || slices.Contains(directories, resolved) {
				continue
			}
			directories = append(directories, resolved)
		}
		break
	}
	return directories
}

func existingWorkspaceDirectory(candidate string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(candidate))
	if err != nil {
		return "", err
	}
	// Match workspace.New: the child's session registry canonicalizes through
	// EvalSymlinks, so the desktop must key on the resolved form too. Otherwise
	// macOS /var -> /private/var aliases hash differently on each side and the
	// child rejects resume with "belongs to another workspace".
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(resolved)
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("workspace path is not a directory")
	}
	return absolute, nil
}

func canonicalWorkspacePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}

func workspaceKey(workspace, sessionID string) string {
	if workspace = strings.TrimSpace(workspace); workspace != "" {
		return "workspace:" + canonicalWorkspacePath(workspace)
	}
	return "session:" + sessionID
}

func workspaceName(workspace, sessionID string) string {
	if workspace = strings.TrimSpace(workspace); workspace != "" {
		return filepath.Base(filepath.Clean(workspace))
	}
	return "Session " + shortID(sessionID)
}

func sessionWorkspaceKey(workDir string) string {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(filepath.Clean(workDir)))
	return hex.EncodeToString(digest[:8])
}

func sessionWorkspaceKeyFromSession(session desktopstate.SessionState) string {
	key := strings.TrimSpace(session.WorkspaceKey)
	if len(key) == 16 && isHexString(key) {
		return key
	}
	if strings.HasPrefix(session.ID, "workspace-") {
		parts := strings.Split(session.ID, "-")
		if len(parts) >= 2 && len(parts[1]) == 16 && isHexString(parts[1]) {
			return parts[1]
		}
	}
	return ""
}

func isHexString(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func resolveACPBinary() string {
	executable, _ := os.Executable()
	return resolveACPBinaryFor(os.Getenv("PROTONMAN_BINARY"), executable, func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	})
}

func resolveACPBinaryFor(override, executable string, isFile func(string) bool) string {
	if override = strings.TrimSpace(override); override != "" {
		return override
	}
	if executable = strings.TrimSpace(executable); executable != "" && isFile != nil {
		candidate := filepath.Join(filepath.Dir(executable), "libexec", "protonman")
		if isFile(candidate) {
			return candidate
		}
		sibling := filepath.Join(filepath.Dir(executable), "protonman")
		if isFile(sibling) {
			return sibling
		}
	}
	return "protonman"
}

func isACPMethodNotFound(err error) bool {
	if err == nil {
		return false
	}
	var rpcError *acpclient.RPCError
	if errors.As(err, &rpcError) && rpcError.Code == -32601 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "-32601") || strings.Contains(msg, "method not found")
}

func waitForReconnect(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextReconnectDelay(delay time.Duration) time.Duration {
	delay *= 2
	if delay > reconnectMaxDelay {
		return reconnectMaxDelay
	}
	return delay
}

func compactError(err error) string {
	message := strings.TrimSpace(err.Error())
	runes := []rune(message)
	if len(runes) <= 120 {
		return message
	}
	return string(runes[:117]) + "…"
}

func shortID(id string) string {
	runes := []rune(id)
	if len(runes) <= 8 {
		return id
	}
	return string(runes[:8])
}
