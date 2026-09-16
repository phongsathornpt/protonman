//go:build desktop

package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type sessionItem struct {
	ID            string `json:"sessionId"`
	AgentID       string `json:"agentId"`
	Title         string `json:"title"`
	Cwd           string `json:"cwd"`
	WorkspaceKey  string `json:"workspaceKey"`
	WorkspaceName string `json:"workspaceName"`
}

type application struct {
	ctx context.Context

	client        *acpclient.Client // compatibility alias for the active agent
	agent         agentProfile
	profiles      map[string]agentProfile
	clients       map[string]*acpclient.Client
	activeAgentID string
	desktopApp    fyne.App

	mu                    sync.Mutex
	state                 desktopstate.State
	sidebarRows           []sidebarRow
	sidebarQuery          string
	collapsedWorkspaces   map[string]bool
	transcripts           map[string]*strings.Builder
	conversationSessionID string
	permissionWaiters     map[string]chan string
	preferences           fyne.Preferences

	status             *widget.Label
	list               *widget.List
	sessionSearch      *widget.Entry
	sidebarEmpty       *widget.Label
	chat               *widget.RichText
	conversationScroll fyne.CanvasObject
	composer           *widget.Entry
	send               *widget.Button
	stop               *widget.Button
	sessionTitle       *widget.Label
	sessionMeta        *widget.Label
	contextToggle      *widget.Button
	contextDrawer      *fyne.Container
	contextContent     *widget.RichText
	runtimeSummary     *widget.Button
	runtimePanel       *fyne.Container
	permissionInbox    *widget.Button
	permissionPanel    *fyne.Container
	permissionTitle    *widget.Label
	permissionDetail   *widget.Label
	permissionActions  *fyne.Container

	modelProvider        *widget.Entry
	modelID              *widget.Entry
	applyModel           *widget.Button
	reasoningSelect      *widget.Select
	lowSelect            *widget.Select
	agentSelect          *widget.Select
	agentSettingsButton  *widget.Button
	agentSettingsPanel   *fyne.Container
	agentSettingsSummary *widget.Label
	agentIDEntry         *widget.Entry
	agentNameEntry       *widget.Entry
	agentCommandEntry    *widget.Entry
	agentArgsEntry       *widget.Entry
	runtimeSync          bool

	integrationButton    *widget.Button
	integrationPanel     *fyne.Container
	integrationSummary   *widget.Label
	integrationName      *widget.Entry
	integrationCommand   *widget.Entry
	integrationArgs      *widget.Entry
	integrationEnv       *widget.Entry
	integrationSave      *widget.Button
	integrationRemove    *widget.Button
	integrationReconnect *widget.Button
}

// Run starts Protonman Desktop. The desktop is deliberately a thin ACP client;
// the Protonman CLI remains the single runtime for sessions, tools and models.
func Run(ctx context.Context) error {
	desktopApp := app.NewWithID("ai.protonman.desktop")
	profiles, err := resolveAgentProfilesForPreferences(desktopApp.Preferences())
	if err != nil {
		return err
	}
	desktopApp.Settings().SetTheme(theme.DarkTheme())
	window := desktopApp.NewWindow("Protonman")
	window.Resize(fyne.NewSize(1220, 780))

	profileMap := make(map[string]agentProfile, len(profiles))
	profileIDs := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		profileMap[profile.ID] = profile
		profileIDs = append(profileIDs, profile.ID)
	}
	ui := &application{
		ctx:                 ctx,
		desktopApp:          desktopApp,
		profiles:            profileMap,
		clients:             make(map[string]*acpclient.Client),
		activeAgentID:       profileIDs[0],
		transcripts:         make(map[string]*strings.Builder),
		collapsedWorkspaces: make(map[string]bool),
		permissionWaiters:   make(map[string]chan string),
		preferences:         desktopApp.Preferences(),
	}
	ui.agentSelect = widget.NewSelect(profileIDs, func(value string) {
		ui.mu.Lock()
		if _, ok := ui.profiles[value]; ok {
			ui.activeAgentID = value
		}
		ui.mu.Unlock()
		ui.populateACPAgentEditor(value)
		ui.renderACPAgentSettings()
		ui.setStatus("ACP agent selected · " + value)
	})
	ui.initDesktopControls()
	// Select invokes its callback synchronously, so all callback targets must
	// exist before selecting the initial value.
	ui.agentSelect.SetSelected(profileIDs[0])
	window.SetContent(ui.buildDesktopShell())

	go ui.connect()
	window.ShowAndRun()
	if client := ui.currentClient(); client != nil {
		_ = client.Close()
	}
	ui.mu.Lock()
	for agentID, client := range ui.clients {
		if agentID != ui.activeAgentID {
			_ = client.Close()
		}
	}
	ui.mu.Unlock()
	return nil
}

func (a *application) connect() {
	a.superviseConnection()
}

func (a *application) refreshSessions() {
	a.mu.Lock()
	clients := make(map[string]*acpclient.Client, len(a.clients))
	for agentID, client := range a.clients {
		clients[agentID] = client
	}
	a.mu.Unlock()
	for agentID, client := range clients {
		a.refreshSessionsFor(agentID, client)
	}
}

func (a *application) refreshSessionsFor(agentID string, client *acpclient.Client) {
	if client == nil {
		return
	}
	var result struct {
		Sessions []sessionItem `json:"sessions"`
	}
	if err := client.Call(a.ctx, "session/list", map[string]any{}, &result); err != nil {
		if isACPMethodNotFound(err) {
			// session/list is not required for an ACP agent. Keep the local
			// session index; new sessions are inserted from session/new below.
			return
		}
		if a.clientIsCurrent(client) {
			a.setStatus("Session list failed · " + err.Error())
		}
		return
	}

	a.mu.Lock()
	old := make(map[string]desktopstate.SessionState, len(a.state.Sessions))
	for _, session := range a.state.Sessions {
		old[session.ID] = session
	}
	sessions := make([]desktopstate.SessionState, 0, len(result.Sessions))
	for _, session := range result.Sessions {
		projected := desktopstate.SessionState{
			ID:            session.ID,
			AgentID:       strings.TrimSpace(session.AgentID),
			Title:         session.Title,
			Workspace:     session.Cwd,
			WorkspaceKey:  strings.TrimSpace(session.WorkspaceKey),
			WorkspaceName: strings.TrimSpace(session.WorkspaceName),
			Status:        desktopstate.TaskIdle,
		}
		if projected.AgentID == "" {
			projected.AgentID = agentID
		}
		if previous, ok := old[session.ID]; ok {
			projected.Status = previous.Status
			projected.Timeline = previous.Timeline
			projected.Subagents = previous.Subagents
			projected.Context = previous.Context
			projected.Runtime = previous.Runtime
			if strings.TrimSpace(projected.Workspace) == "" {
				projected.Workspace = previous.Workspace
			}
			if projected.WorkspaceKey == "" {
				projected.WorkspaceKey = previous.WorkspaceKey
			}
			if projected.WorkspaceName == "" {
				projected.WorkspaceName = previous.WorkspaceName
			}
		}
		projected.Workspace = a.resolveWorkspacePath(projected.WorkspaceKey, projected.Workspace)
		if projected.WorkspaceName == "" {
			projected.WorkspaceName = inferredWorkspaceName(projected.Title, projected.ID)
		}
		sessions = append(sessions, projected)
		if a.transcripts[session.ID] == nil {
			a.transcripts[session.ID] = &strings.Builder{}
		}
	}
	for _, previous := range old {
		if previous.AgentID != "" && previous.AgentID != agentID {
			sessions = append(sessions, previous)
		}
	}
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionsReplaced, Sessions: sessions})
	a.rebuildSidebarRowsLocked()
	a.mu.Unlock()
	fyne.Do(func() {
		a.list.Refresh()
		a.refreshSidebarEmptyState()
	})
	a.refreshActiveView()
	a.refreshPermissionView()
}

func (a *application) newSession() {
	client := a.currentClient()
	if client == nil {
		return
	}
	a.mu.Lock()
	agentID := a.activeAgentID
	a.mu.Unlock()
	cwd := a.activeWorkspacePath()
	if cwd == "" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			a.setStatus("New session failed · determine workspace: " + err.Error())
			return
		}
		cwd = validWorkspacePath(workingDirectory)
	}
	if cwd == "" {
		a.setStatus("New session failed · workspace path is unavailable")
		return
	}
	go func() {
		var result struct {
			SessionID string `json:"sessionId"`
		}
		params := map[string]any{"cwd": cwd}
		if servers := a.mcpServersPayload(); len(servers) > 0 {
			params["mcpServers"] = servers
		}
		if err := client.Call(a.ctx, "session/new", params, &result); err != nil {
			if a.clientIsCurrent(client) {
				a.setStatus("New session failed · " + err.Error())
			}
			return
		}
		if !a.clientIsCurrent(client) {
			return
		}
		markSessionHistoryLoaded(a, result.SessionID)
		a.mu.Lock()
		// The remote ACP server owns the session ID; Desktop owns the routing
		// association needed when several ACP processes are connected.
		found := false
		for i := range a.state.Sessions {
			if a.state.Sessions[i].ID == result.SessionID {
				a.state.Sessions[i].AgentID = agentID
				found = true
			}
		}
		if !found {
			a.state.Sessions = append(a.state.Sessions, desktopstate.SessionState{
				ID: result.SessionID, AgentID: agentID, Title: "Session " + shortID(result.SessionID),
				Workspace: cwd, Status: desktopstate.TaskIdle,
			})
			a.rebuildSidebarRowsLocked()
		}
		a.mu.Unlock()
		a.refreshSessions()
		a.mu.Lock()
		a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionSelected, SessionID: result.SessionID})
		index := sidebarRowIndexForSession(a.sidebarRows, result.SessionID)
		a.mu.Unlock()
		if index >= 0 {
			fyne.Do(func() { a.list.Select(widget.ListItemID(index)) })
		}
		a.refreshActiveView()
		a.refreshPermissionView()
	}()
}

func isACPMethodNotFound(err error) bool {
	var rpcErr *acpclient.RPCError
	return errors.As(err, &rpcErr) && rpcErr.Code == -32601
}

func (a *application) sendPrompt() {
	text := strings.TrimSpace(a.composer.Text)
	if text == "" {
		return
	}
	a.mu.Lock()
	sessionID := a.state.ActiveSessionID
	client := a.clientForSessionLocked(sessionID)
	if sessionID == "" || a.sessionBusyLocked(sessionID) || client == nil {
		a.mu.Unlock()
		if sessionID != "" && client == nil {
			a.setStatus("ACP agent is disconnected for this session")
		}
		return
	}
	workspace := ""
	for _, session := range a.state.Sessions {
		if session.ID == sessionID {
			workspace = validWorkspacePath(session.Workspace)
			break
		}
	}
	if workspace == "" {
		a.mu.Unlock()
		a.setStatus("Cannot run session · workspace path is unavailable; open the workspace again instead of falling back to the Desktop process directory")
		return
	}
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventPromptStarted, SessionID: sessionID})
	a.mu.Unlock()

	a.composer.SetText("")
	a.appendTranscript(sessionID, "\n\n> "+text+"\n\n")
	fyne.Do(func() { a.list.Refresh() })

	go func() {
		resumeParams := map[string]any{"sessionId": sessionID, "cwd": workspace}
		if servers := a.mcpServersPayload(); len(servers) > 0 {
			resumeParams["mcpServers"] = servers
		}
		err := client.Call(a.ctx, "session/resume", resumeParams, nil)
		var result struct {
			StopReason string `json:"stopReason"`
		}
		if err == nil {
			err = client.Call(a.ctx, "session/prompt", map[string]any{
				"sessionId": sessionID,
				"prompt":    []map[string]any{{"type": "text", "text": text}},
			}, &result)
		}

		if !a.clientIsCurrent(client) {
			return
		}
		a.mu.Lock()
		kind := desktopstate.EventPromptCompleted
		if err != nil {
			kind = desktopstate.EventPromptFailed
		}
		a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: kind, SessionID: sessionID})
		a.mu.Unlock()
		if err != nil {
			a.appendTranscript(sessionID, "\n\n**Error:** "+err.Error()+"\n")
		}
		a.notifyTurnFinished(sessionID, err)
		fyne.Do(func() { a.list.Refresh() })
		a.refreshActiveView()
	}()
}

func (a *application) cancelPrompt() {
	a.mu.Lock()
	sessionID := a.state.ActiveSessionID
	client := a.clientForSessionLocked(sessionID)
	busy := a.sessionBusyLocked(sessionID)
	a.mu.Unlock()
	if sessionID == "" || !busy {
		return
	}
	go func() {
		if err := client.Call(a.ctx, "session/cancel", map[string]any{"sessionId": sessionID}, nil); err != nil && a.clientIsCurrent(client) {
			a.setStatus("Cancel failed · " + err.Error())
		}
	}()
}

func (a *application) refreshPermissionView() {
	a.mu.Lock()
	activeID := a.state.ActiveSessionID
	count := len(a.state.PermissionInbox)
	var current *desktopstate.PermissionRequest
	for i := range a.state.PermissionInbox {
		if a.state.PermissionInbox[i].SessionID == activeID {
			item := a.state.PermissionInbox[i]
			current = &item
			break
		}
	}
	a.mu.Unlock()

	fyne.Do(func() {
		a.permissionInbox.SetText(fmt.Sprintf("Permissions %d", count))
		if count == 0 {
			a.permissionInbox.Disable()
		} else {
			a.permissionInbox.Enable()
		}
		if current == nil {
			a.permissionPanel.Hide()
			return
		}
		a.permissionTitle.SetText("Permission required · " + current.Title)
		detail := current.Detail
		if strings.TrimSpace(detail) == "" {
			detail = "Review the requested tool action before continuing."
		}
		a.permissionDetail.SetText(detail)
		a.permissionActions.Objects = nil
		for _, option := range current.Options {
			option := option
			requestID := current.RequestID
			a.permissionActions.Add(widget.NewButton(option.Name, func() {
				a.resolvePermission(requestID, option.ID)
			}))
		}
		a.permissionActions.Refresh()
		a.permissionPanel.Show()
	})
}

func (a *application) selectNextPermission() {
	a.mu.Lock()
	if len(a.state.PermissionInbox) == 0 {
		a.mu.Unlock()
		return
	}
	sessionID := a.state.PermissionInbox[0].SessionID
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionSelected, SessionID: sessionID})
	a.sidebarQuery = ""
	a.rebuildSidebarRowsLocked()
	index := sidebarRowIndexForSession(a.sidebarRows, sessionID)
	a.mu.Unlock()
	fyne.Do(func() {
		if a.sessionSearch != nil && a.sessionSearch.Text != "" {
			a.sessionSearch.SetText("")
		}
		if index >= 0 {
			a.list.Select(widget.ListItemID(index))
		}
	})
	a.refreshActiveView()
	a.refreshPermissionView()
}

func (a *application) sessionBusyLocked(sessionID string) bool {
	for _, session := range a.state.Sessions {
		if session.ID != sessionID {
			continue
		}
		switch session.Status {
		case desktopstate.TaskQueued, desktopstate.TaskRunning, desktopstate.TaskWaitingPermission, desktopstate.TaskWaitingUser:
			return true
		default:
			return false
		}
	}
	return false
}

func (a *application) setStatus(text string) {
	fyne.Do(func() { a.status.SetText(text) })
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
