//go:build desktop

package desktop

import (
	"context"
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
	Title         string `json:"title"`
	Cwd           string `json:"cwd"`
	WorkspaceKey  string `json:"workspaceKey"`
	WorkspaceName string `json:"workspaceName"`
}

type application struct {
	ctx context.Context

	client            *acpclient.Client
	agentCapabilities acpclient.AgentCapabilities
	desktopApp        fyne.App
	window            fyne.Window

	mu                    sync.Mutex
	state                 desktopstate.State
	sidebarRows           []sidebarRow
	sidebarQuery          string
	transcripts           map[string]*strings.Builder
	conversationSessionID string
	permissionWaiters     map[string]chan string
	sessionConfigOptions  map[string][]acpclient.SessionConfigOption
	attachments           []composerAttachment
	preferences           fyne.Preferences

	status             *widget.Label
	list               *widget.List
	sessionSearch      *widget.Entry
	chat               *widget.RichText
	conversationScroll fyne.CanvasObject
	composer           *widget.Entry
	attachmentStrip    *fyne.Container
	attachButton       *widget.Button
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

	modelProvider   *widget.Entry
	modelSelect     *widget.Select
	reasoningSelect *widget.Select
	lowSelect       *widget.Select
	runtimeSync     bool

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
	desktopApp.Settings().SetTheme(theme.DarkTheme())
	window := desktopApp.NewWindow("Protonman")
	window.Resize(fyne.NewSize(1220, 780))

	ui := &application{
		ctx:                  ctx,
		desktopApp:           desktopApp,
		window:               window,
		transcripts:          make(map[string]*strings.Builder),
		permissionWaiters:    make(map[string]chan string),
		sessionConfigOptions: make(map[string][]acpclient.SessionConfigOption),
		preferences:          desktopApp.Preferences(),
	}
	ui.initDesktopControls()
	window.SetContent(ui.buildDesktopShell())

	go ui.connect()
	window.ShowAndRun()
	if client := ui.currentClient(); client != nil {
		_ = client.Close()
	}
	return nil
}

func (a *application) connect() {
	a.superviseConnection()
}

func (a *application) refreshSessions() {
	client := a.currentClient()
	if client == nil {
		return
	}
	var result struct {
		Sessions []sessionItem `json:"sessions"`
	}
	if err := client.Call(a.ctx, "session/list", map[string]any{}, &result); err != nil {
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
			Title:         session.Title,
			Workspace:     session.Cwd,
			WorkspaceKey:  strings.TrimSpace(session.WorkspaceKey),
			WorkspaceName: strings.TrimSpace(session.WorkspaceName),
			Status:        desktopstate.TaskIdle,
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
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionsReplaced, Sessions: sessions})
	a.rebuildSidebarRowsLocked()
	a.mu.Unlock()
	fyne.Do(func() { a.list.Refresh() })
	a.refreshActiveView()
	a.refreshPermissionView()
}

func (a *application) newSession() {
	client := a.currentClient()
	if client == nil {
		return
	}
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
		var result acpclient.SessionNewResult
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
		a.refreshSessions()
		a.mu.Lock()
		a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionSelected, SessionID: result.SessionID})
		index := sidebarRowIndexForSession(a.sidebarRows, result.SessionID)
		a.mu.Unlock()
		a.applySessionConfigOptions(result.SessionID, result.ConfigOptions)
		if index >= 0 {
			fyne.Do(func() { a.list.Select(widget.ListItemID(index)) })
		}
		a.refreshActiveView()
		a.refreshPermissionView()
	}()
}

func (a *application) sendPrompt() {
	text := strings.TrimSpace(a.composer.Text)
	client := a.currentClient()
	if client == nil {
		return
	}
	a.mu.Lock()
	attachments := append([]composerAttachment(nil), a.attachments...)
	if text == "" && len(attachments) == 0 {
		a.mu.Unlock()
		return
	}
	sessionID := a.state.ActiveSessionID
	if sessionID == "" || a.sessionBusyLocked(sessionID) {
		a.mu.Unlock()
		return
	}
	workspace := ""
	for _, session := range a.state.Sessions {
		if session.ID == sessionID {
			workspace = validWorkspacePath(session.Workspace)
			break
		}
	}
	resumeSupported := a.agentCapabilities.SessionCapabilities.Resume != nil
	if workspace == "" {
		a.mu.Unlock()
		a.setStatus("Cannot run session · workspace path is unavailable; open the workspace again instead of falling back to the Desktop process directory")
		return
	}
	a.attachments = nil
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventPromptStarted, SessionID: sessionID})
	a.mu.Unlock()

	blocks := a.promptContentBlocks(text, attachments)
	a.composer.SetText("")
	a.renderAttachments()
	display := text
	if len(attachments) > 0 {
		if display != "" {
			display += "\n"
		}
		display += fmt.Sprintf("[%d attachment(s)]", len(attachments))
	}
	a.appendTranscript(sessionID, "\n\n> "+display+"\n\n")
	fyne.Do(func() { a.list.Refresh() })

	go func() {
		var err error
		if resumeSupported {
			resumeParams := map[string]any{"sessionId": sessionID, "cwd": workspace}
			if servers := a.mcpServersPayload(); len(servers) > 0 {
				resumeParams["mcpServers"] = servers
			}
			var resumeResult acpclient.SessionResumeResult
			err = client.Call(a.ctx, "session/resume", resumeParams, &resumeResult)
			if err == nil && a.clientIsCurrent(client) {
				a.applySessionConfigOptions(sessionID, resumeResult.ConfigOptions)
			}
		}
		var result acpclient.SessionPromptResult
		if err == nil {
			err = client.Call(a.ctx, "session/prompt", acpclient.SessionPromptParams{
				SessionID: sessionID,
				Prompt:    blocks,
			}, &result)
		}

		if !a.clientIsCurrent(client) {
			return
		}
		a.mu.Lock()
		kind := desktopstate.EventPromptCompleted
		if err != nil {
			kind = desktopstate.EventPromptFailed
			a.attachments = append(attachments, a.attachments...)
		}
		a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: kind, SessionID: sessionID})
		a.mu.Unlock()
		if err != nil {
			a.appendTranscript(sessionID, "\n\n**Error:** "+err.Error()+"\n")
			a.renderAttachments()
		}
		a.notifyTurnFinished(sessionID, err)
		fyne.Do(func() { a.list.Refresh() })
		a.refreshActiveView()
	}()
}

func (a *application) cancelPrompt() {
	client := a.currentClient()
	if client == nil {
		return
	}
	a.mu.Lock()
	sessionID := a.state.ActiveSessionID
	busy := a.sessionBusyLocked(sessionID)
	a.mu.Unlock()
	if sessionID == "" || !busy {
		return
	}
	go func() {
		if err := client.Notify("session/cancel", map[string]any{"sessionId": sessionID}); err != nil && a.clientIsCurrent(client) {
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
