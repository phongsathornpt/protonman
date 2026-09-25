//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	controllerAgentID       = "protonman"
	acpAgentsEnvironment    = "PROTONMAN_ACP_AGENTS_JSON"
	reconnectInitialDelay   = time.Second
	reconnectMaxDelay       = 8 * time.Second
	reconnectRequestTimeout = 15 * time.Second
	newSessionTimeout       = 30 * time.Second
)

type connectionPhase string

const (
	connectionConnecting   connectionPhase = "connecting"
	connectionConnected    connectionPhase = "connected"
	connectionReconnecting connectionPhase = "reconnecting"
)

type acpSession struct {
	ID                    string   `json:"sessionId"`
	AgentID               string   `json:"agentId"`
	Title                 string   `json:"title"`
	Cwd                   string   `json:"cwd"`
	WorkspaceKey          string   `json:"workspaceKey"`
	WorkspaceName         string   `json:"workspaceName"`
	AdditionalDirectories []string `json:"additionalDirectories"`
}

type controllerSnapshot struct {
	State                 desktopstate.State
	Connection            connectionPhase
	Status                string
	ActiveAgentID         string
	AgentProfiles         []app.ACPAgentProfile
	AgentConnections      map[string]connectionPhase
	AgentConfigOverridden bool
	AgentUpdating         bool
	AgentError            string
	CreatingSession       bool
	HistoryState          historyState
	RuntimeUpdating       bool
	MCPUpdating           bool
	MCPReconnecting       bool
	MCPError              string
	Revision              uint64
}

type controllerSnapshotCache struct {
	valid    bool
	revision uint64
	value    controllerSnapshot
}

type controller struct {
	ctx      context.Context
	cancel   context.CancelFunc
	onChange func()

	sessionLocks  sync.Map
	mu            sync.RWMutex
	state         desktopstate.State
	profiles      map[string]app.ACPAgentProfile
	clients       map[string]*acpclient.Client
	connections   map[string]connectionPhase
	statuses      map[string]string
	activeAgentID string

	histories       map[string]historyState
	historyStaging  map[string][]desktopstate.Event
	messageStreams  map[string]string
	messageSequence map[string]uint64
	permissionWait  map[string]chan string
	contextRefresh  *sessionRefreshTracker
	memoryRefresh   *sessionRefreshTracker
	runtimeRefresh  *sessionRefreshTracker
	runtimeMutation string

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
	snapshotCache   controllerSnapshotCache
}

func newController(parent context.Context, onChange func(), agents app.ACPAgents, integrations app.MCPIntegrations) *controller {
	ctx, cancel := context.WithCancel(parent)
	defaults := []app.ACPAgentProfile{defaultACPAgentProfile()}
	profiles, profileErr := agents.Resolve(ctx, defaults, os.Getenv(acpAgentsEnvironment))
	agentError := ""
	if profileErr != nil {
		profiles = defaults
		agentError = compactError(profileErr)
	}
	instance := &controller{
		ctx:                   ctx,
		cancel:                cancel,
		onChange:              onChange,
		profiles:              make(map[string]app.ACPAgentProfile, len(profiles)),
		clients:               make(map[string]*acpclient.Client, len(profiles)),
		connections:           make(map[string]connectionPhase, len(profiles)),
		statuses:              make(map[string]string, len(profiles)),
		histories:             make(map[string]historyState),
		historyStaging:        make(map[string][]desktopstate.Event),
		messageStreams:        make(map[string]string),
		messageSequence:       make(map[string]uint64),
		permissionWait:        make(map[string]chan string),
		contextRefresh:        newSessionRefreshTracker(contextRefreshInterval),
		memoryRefresh:         newSessionRefreshTracker(memoryRefreshInterval),
		runtimeRefresh:        newSessionRefreshTracker(runtimeRefreshInterval),
		agentProfiles:         agents,
		agentConfigOverridden: strings.TrimSpace(os.Getenv(acpAgentsEnvironment)) != "",
		agentError:            agentError,
		mcpIntegrations:       integrations,
	}
	for _, profile := range profiles {
		instance.profiles[profile.ID] = cloneACPAgentProfile(profile)
		instance.connections[profile.ID] = connectionConnecting
		instance.statuses[profile.ID] = "Starting " + profile.DisplayName + "…"
	}
	instance.activeAgentID = preferredAgentID(profiles)
	if integrations.Available() {
		instance.loadMCPIntegrations()
	}
	go instance.run()
	return instance
}

func (c *controller) close() {
	c.cancel()
	c.mu.Lock()
	clients := make([]*acpclient.Client, 0, len(c.clients))
	for _, client := range c.clients {
		clients = append(clients, client)
	}
	clear(c.clients)
	c.mu.Unlock()
	for _, client := range clients {
		_ = client.Close()
	}
}

func (c *controller) snapshot() controllerSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snapshotCache.valid && c.snapshotCache.revision == c.revision {
		return c.snapshotCache.value
	}
	snapshot := controllerSnapshot{
		State:                 desktopstate.ClonePresentationState(c.state),
		Connection:            c.connections[c.activeAgentID],
		Status:                c.statuses[c.activeAgentID],
		ActiveAgentID:         c.activeAgentID,
		AgentProfiles:         cloneACPAgentProfiles(c.profiles),
		AgentConnections:      make(map[string]connectionPhase, len(c.connections)),
		AgentConfigOverridden: c.agentConfigOverridden,
		AgentUpdating:         c.agentMutation,
		AgentError:            c.agentError,
		CreatingSession:       c.creatingSession,
		HistoryState:          historyStateUnloaded,
		RuntimeUpdating:       c.runtimeMutation != "",
		MCPUpdating:           c.mcpMutation,
		MCPReconnecting:       c.mcpReconnect,
		MCPError:              c.mcpError,
		Revision:              c.revision,
	}
	for agentID, phase := range c.connections {
		snapshot.AgentConnections[agentID] = phase
	}
	if state, ok := c.histories[snapshot.State.ActiveSessionID]; ok {
		snapshot.HistoryState = state
	}
	c.snapshotCache.valid = true
	c.snapshotCache.revision = c.revision
	c.snapshotCache.value = snapshot
	return snapshot
}

func (c *controller) selectSession(sessionID string) {
	c.mu.Lock()
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventSessionSelected,
		SessionID: sessionID,
	})
	for _, session := range c.state.Sessions {
		if session.ID == c.state.ActiveSessionID {
			c.state.ActiveProjectID = session.ProjectID
			if strings.TrimSpace(session.AgentID) != "" {
				c.activeAgentID = session.AgentID
			}
			break
		}
	}
	c.revision++
	activeSessionID := c.state.ActiveSessionID
	c.mu.Unlock()
	c.notify()
	c.loadSessionHistory(activeSessionID)
	c.refreshActiveSession(true)
}

func (c *controller) newSession() {
	c.mu.Lock()
	agentID := c.agentForProjectLocked(c.state.ActiveProjectID)
	client := c.clients[agentID]
	if c.connections[agentID] != connectionConnected || client == nil || c.creatingSession {
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
	additionalDirectories := c.projectAdditionalDirectoriesLocked(c.state.ActiveProjectID, workspace)
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
		callCtx, cancel := context.WithTimeout(c.ctx, newSessionTimeout)
		defer cancel()
		var result struct {
			SessionID string `json:"sessionId"`
		}
		err := client.Call(callCtx, "session/new", params, &result)
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
		c.state.ActiveSessionID = result.SessionID
		c.state.ActiveProjectID = workspaceKey(workspace, result.SessionID)
		c.histories[result.SessionID] = historyStateLoaded
		c.clearMessageStreamsLocked(result.SessionID)
		c.setProjectDefaultAgentLocked(c.state.ActiveProjectID, agentID)
		c.statuses[agentID] = "Session created"
		c.revision++
		c.mu.Unlock()
		c.refreshActiveSession(true)
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
		c.setAgentConnection(profile.ID, connectionConnecting, "Starting "+profile.DisplayName+"…")
		var startedClient atomic.Pointer[acpclient.Client]
		client, err := acpclient.StartCommand(c.ctx, commandSpecForACPAgent(profile), func(event acpclient.Event) {
			if current := startedClient.Load(); current != nil {
				c.handleACPEventForAgent(profile.ID, current, event)
			}
		})
		if err != nil {
			c.setAgentConnection(profile.ID, connectionReconnecting, "Start failed · "+compactError(err))
			if !waitForReconnect(c.ctx, delay) {
				return
			}
			delay = nextReconnectDelay(delay)
			continue
		}
		startedClient.Store(client)

		if err := initializeACP(c.ctx, client); err != nil {
			_ = client.Close()
			if c.ctx.Err() != nil {
				return
			}
			c.setAgentConnection(profile.ID, connectionReconnecting, "Connection failed · "+compactError(err))
			if !waitForReconnect(c.ctx, delay) {
				return
			}
			delay = nextReconnectDelay(delay)
			continue
		}
		client.SetRequestHandler(func(requestCtx context.Context, request acpclient.Request) (any, error) {
			return c.handlePermissionRequestForAgent(profile.ID, client, requestCtx, request)
		})

		c.setClient(profile.ID, client)
		delay = reconnectInitialDelay
		c.resumeKnownSessions(profile.ID, client)
		if err := c.refreshSessions(profile.ID, client); err != nil {
			if isACPMethodNotFound(err) {
				c.setAgentStatus(profile.ID, "Connected · session list unavailable")
			} else {
				c.setAgentStatus(profile.ID, "Connected · session refresh failed")
			}
		}

		select {
		case <-c.ctx.Done():
			_ = client.Close()
			return
		case <-client.Done():
		}

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
	c.refreshActiveSession(true)
	return nil
}

func (c *controller) setAgentConnection(agentID string, phase connectionPhase, status string) {
	c.mu.Lock()
	c.connections[agentID] = phase
	c.statuses[agentID] = status
	if agentID == c.activeAgentID {
		c.mcpReconnect = c.mcpReconnect && phase != connectionConnected
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
	c.connections[agentID] = connectionConnected
	status := "Connected · ACP v1"
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
}

func (c *controller) markAgentDisconnected(agentID string, client *acpclient.Client) {
	c.mu.Lock()
	if c.clients[agentID] != client {
		c.mu.Unlock()
		return
	}
	delete(c.clients, agentID)
	c.connections[agentID] = connectionReconnecting
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

func initializeACP(ctx context.Context, client *acpclient.Client) error {
	callCtx, cancel := context.WithTimeout(ctx, reconnectRequestTimeout)
	defer cancel()
	var result struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	err := client.Call(callCtx, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientInfo":         map[string]any{"name": "protonman-desktop-gio", "title": "Protonman Desktop"},
		"clientCapabilities": map[string]any{},
	}, &result)
	if err != nil {
		return err
	}
	if result.ProtocolVersion != 1 {
		return fmt.Errorf("unsupported ACP v%d", result.ProtocolVersion)
	}
	return nil
}

func projectSessions(current desktopstate.State, agentID string, remoteSessions []acpSession) desktopstate.State {
	previous := make(map[string]desktopstate.SessionState, len(current.Sessions))
	for _, session := range current.Sessions {
		if session.AgentID == agentID || session.AgentID == "" && agentID == controllerAgentID {
			previous[session.ID] = session
		}
	}
	sessions := make([]desktopstate.SessionState, 0, len(remoteSessions)+len(current.Sessions))
	for _, session := range current.Sessions {
		if session.AgentID != agentID && !(session.AgentID == "" && agentID == controllerAgentID) {
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
	}
	for _, session := range sessions {
		if session.ID == next.ActiveSessionID {
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
		ID:            sessionID,
		AgentID:       agentID,
		ProjectID:     workspaceKey(workspace, sessionID),
		Title:         "Session " + shortID(sessionID),
		Workspace:     workspace,
		WorkspaceKey:  workspaceKey(workspace, sessionID),
		WorkspaceName: workspaceName(workspace, sessionID),
		Status:        desktopstate.TaskIdle,
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
	}
	return "protonman"
}

func isACPMethodNotFound(err error) bool {
	var rpcError *acpclient.RPCError
	return errors.As(err, &rpcError) && rpcError.Code == -32601
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
