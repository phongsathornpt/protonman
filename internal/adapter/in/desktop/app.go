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
	"fyne.io/fyne/v2/container"
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

	client     *acpclient.Client
	desktopApp fyne.App

	mu                sync.Mutex
	state             desktopstate.State
	sidebarRows       []sidebarRow
	transcripts       map[string]*strings.Builder
	permissionWaiters map[string]chan string
	preferences       fyne.Preferences

	status               *widget.Label
	list                 *widget.List
	chat                 *widget.RichText
	composer             *widget.Entry
	send                 *widget.Button
	stop                 *widget.Button
	permissionInbox      *widget.Button
	permissionPanel      *fyne.Container
	permissionTitle      *widget.Label
	permissionDetail     *widget.Label
	permissionActions    *fyne.Container
	modelProvider        *widget.Entry
	modelID              *widget.Entry
	applyModel           *widget.Button
	reasoningSelect      *widget.Select
	lowSelect            *widget.Select
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
	a := app.NewWithID("ai.protonman.desktop")
	a.Settings().SetTheme(theme.DarkTheme())
	w := a.NewWindow("Protonman")
	w.Resize(fyne.NewSize(1220, 780))

	ui := &application{
		ctx:               ctx,
		desktopApp:        a,
		transcripts:       make(map[string]*strings.Builder),
		permissionWaiters: make(map[string]chan string),
		preferences:       a.Preferences(),
	}
	ui.status = widget.NewLabel("Connecting to Protonman…")
	ui.chat = widget.NewRichTextFromMarkdown("")
	ui.composer = widget.NewEntry()
	ui.composer.SetPlaceHolder("Message protonMAN…")
	ui.send = widget.NewButton("Send", ui.sendPrompt)
	ui.stop = widget.NewButtonWithIcon("", theme.MediaStopIcon(), ui.cancelPrompt)
	ui.permissionInbox = widget.NewButton("Permissions 0", ui.selectNextPermission)
	ui.permissionTitle = widget.NewLabelWithStyle("Permission required", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ui.permissionDetail = widget.NewLabel("")
	ui.permissionDetail.Wrapping = fyne.TextWrapWord
	ui.permissionActions = container.NewHBox()
	ui.permissionPanel = container.NewVBox(
		widget.NewSeparator(),
		ui.permissionTitle,
		ui.permissionDetail,
		ui.permissionActions,
		widget.NewSeparator(),
	)
	ui.permissionPanel.Hide()
	ui.send.Disable()
	ui.stop.Disable()
	ui.modelProvider = widget.NewEntry()
	ui.modelProvider.SetPlaceHolder("provider")
	ui.modelID = widget.NewEntry()
	ui.modelID.SetPlaceHolder("model")
	ui.applyModel = widget.NewButton("Apply", ui.setRuntimeModel)
	ui.reasoningSelect = widget.NewSelect([]string{"auto", "none", "minimal", "low", "medium", "high", "xhigh", "max"}, func(value string) {
		if !ui.runtimeSync {
			ui.setRuntimeReasoning(value)
		}
	})
	ui.lowSelect = widget.NewSelect([]string{"auto", "on", "off"}, func(value string) {
		if !ui.runtimeSync {
			ui.setRuntimeLowConcurrency(value)
		}
	})
	ui.initIntegrationControls()

	ui.list = widget.NewList(
		func() int {
			ui.mu.Lock()
			defer ui.mu.Unlock()
			return len(ui.sidebarRows)
		},
		func() fyne.CanvasObject {
			title := newNerdIconText(iconSession, "Session", fyne.TextStyle{}, false)
			subtitle := newNerdIconText(iconReady, "ready", fyne.TextStyle{}, true)
			return container.NewVBox(title, subtitle)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			ui.mu.Lock()
			if id < 0 || id >= len(ui.sidebarRows) {
				ui.mu.Unlock()
				return
			}
			row := ui.sidebarRows[id]
			var session desktopstate.SessionState
			if row.Kind == sidebarSessionRow {
				for _, candidate := range ui.state.Sessions {
					if candidate.ID == row.SessionID {
						session = candidate
						break
					}
				}
			}
			ui.mu.Unlock()

			box := object.(*fyne.Container)
			title := box.Objects[0].(*fyne.Container)
			subtitle := box.Objects[1].(*fyne.Container)
			if row.Kind == sidebarWorkspaceRow {
				setNerdIconText(title, iconFolder, row.WorkspaceName)
				setNerdIconText(subtitle, iconSession, fmt.Sprintf("%d sessions", row.SessionCount))
				return
			}
			if strings.TrimSpace(session.Title) == "" {
				setNerdIconText(title, iconSession, "Session "+shortID(session.ID))
			} else {
				setNerdIconText(title, iconSession, session.Title)
			}
			status := "ready"
			if session.Status != desktopstate.TaskIdle {
				status = string(session.Status)
			}
			setNerdIconText(subtitle, taskStatusIcon(session.Status), status)
		},
	)
	ui.list.OnSelected = func(id widget.ListItemID) {
		ui.mu.Lock()
		if id < 0 || id >= len(ui.sidebarRows) {
			ui.mu.Unlock()
			return
		}
		row := ui.sidebarRows[id]
		if row.Kind != sidebarSessionRow {
			activeIndex := sidebarRowIndexForSession(ui.sidebarRows, ui.state.ActiveSessionID)
			ui.mu.Unlock()
			if activeIndex >= 0 {
				fyne.Do(func() { ui.list.Select(widget.ListItemID(activeIndex)) })
			}
			return
		}
		ui.state = desktopstate.Reduce(ui.state, desktopstate.Event{Kind: desktopstate.EventSessionSelected, SessionID: row.SessionID})
		ui.mu.Unlock()
		ui.refreshActiveView()
		ui.refreshPermissionView()
	}

	search := widget.NewEntry()
	search.SetPlaceHolder("Search")
	newTask := widget.NewButtonWithIcon("", theme.ContentAddIcon(), ui.newSession)
	sidebarHeader := container.NewBorder(nil, nil, nil, newTask, newNerdIconText(iconRocket, "protonMAN", fyne.TextStyle{Bold: true}, false))
	sidebar := container.NewBorder(
		container.NewVBox(sidebarHeader, search),
		container.NewVBox(widget.NewSeparator(), ui.integrationButton, ui.integrationPanel, ui.permissionInbox, widget.NewLabel("Desktop via ACP")),
		nil,
		nil,
		ui.list,
	)

	runtimeControls := container.NewHBox(ui.modelProvider, ui.modelID, ui.applyModel, widget.NewLabel("Reasoning"), ui.reasoningSelect, widget.NewLabel("Low"), ui.lowSelect)
	headerActions := container.NewHBox(ui.stop, ui.status)
	header := container.NewBorder(runtimeControls, nil, nil, headerActions,
		container.NewVBox(
			newNerdIconText(iconRocket, "protonMAN", fyne.TextStyle{Bold: true}, false),
			widget.NewLabel("Coding agent · ACP"),
		),
	)
	composer := container.NewBorder(nil, nil, nil, ui.send, ui.composer)
	conversationBody := container.NewBorder(ui.permissionPanel, nil, nil, nil, container.NewVScroll(ui.chat))
	conversation := container.NewBorder(header, composer, nil, nil, conversationBody)

	split := container.NewHSplit(sidebar, conversation)
	split.Offset = 0.29
	w.SetContent(container.NewPadded(split))

	go ui.connect()
	w.ShowAndRun()
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
		if projected.WorkspaceName == "" {
			projected.WorkspaceName = inferredWorkspaceName(projected.Title, projected.ID)
		}
		sessions = append(sessions, projected)
		if a.transcripts[session.ID] == nil {
			a.transcripts[session.ID] = &strings.Builder{}
		}
	}
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionsReplaced, Sessions: sessions})
	a.sidebarRows = buildSidebarRows(a.state.Sessions)
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
	cwd, _ := os.Getwd()
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

func (a *application) sendPrompt() {
	text := strings.TrimSpace(a.composer.Text)
	client := a.currentClient()
	if text == "" || client == nil {
		return
	}
	a.mu.Lock()
	sessionID := a.state.ActiveSessionID
	if sessionID == "" || a.sessionBusyLocked(sessionID) {
		a.mu.Unlock()
		return
	}
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventPromptStarted, SessionID: sessionID})
	a.mu.Unlock()

	a.composer.SetText("")
	a.appendTranscript(sessionID, "\n\n> "+text+"\n\n")
	fyne.Do(func() { a.list.Refresh() })

	go func() {
		var result struct {
			StopReason string `json:"stopReason"`
		}
		err := client.Call(a.ctx, "session/prompt", map[string]any{
			"sessionId": sessionID,
			"prompt":    []map[string]any{{"type": "text", "text": text}},
		}, &result)

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
	index := sidebarRowIndexForSession(a.sidebarRows, sessionID)
	a.mu.Unlock()
	if index >= 0 {
		fyne.Do(func() { a.list.Select(widget.ListItemID(index)) })
	}
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
