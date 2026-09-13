//go:build desktop

package desktop

import (
	"context"
	"encoding/json"
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
	ID    string `json:"sessionId"`
	Title string `json:"title"`
	Cwd   string `json:"cwd"`
}

type application struct {
	ctx context.Context

	client *acpclient.Client

	mu          sync.Mutex
	state       desktopstate.State
	transcripts map[string]*strings.Builder

	status   *widget.Label
	list     *widget.List
	chat     *widget.RichText
	composer *widget.Entry
	send     *widget.Button
	stop     *widget.Button
}

// Run starts Protonman Desktop. The desktop is deliberately a thin ACP client;
// the Protonman CLI remains the single runtime for sessions, tools and models.
func Run(ctx context.Context) error {
	a := app.NewWithID("ai.protonman.desktop")
	a.Settings().SetTheme(theme.DarkTheme())
	w := a.NewWindow("Protonman")
	w.Resize(fyne.NewSize(1220, 780))

	ui := &application{ctx: ctx, transcripts: make(map[string]*strings.Builder)}
	ui.status = widget.NewLabel("Connecting to Protonman…")
	ui.chat = widget.NewRichTextFromMarkdown("")
	ui.composer = widget.NewEntry()
	ui.composer.SetPlaceHolder("Message protonMAN…")
	ui.send = widget.NewButton("Send", ui.sendPrompt)
	ui.stop = widget.NewButtonWithIcon("", theme.MediaStopIcon(), ui.cancelPrompt)
	ui.send.Disable()
	ui.stop.Disable()

	ui.list = widget.NewList(
		func() int {
			ui.mu.Lock()
			defer ui.mu.Unlock()
			return len(ui.state.Sessions)
		},
		func() fyne.CanvasObject {
			title := widget.NewLabel("Session")
			subtitle := widget.NewLabel("workspace")
			subtitle.Importance = widget.LowImportance
			return container.NewVBox(title, subtitle)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			ui.mu.Lock()
			if id < 0 || id >= len(ui.state.Sessions) {
				ui.mu.Unlock()
				return
			}
			s := ui.state.Sessions[id]
			ui.mu.Unlock()
			box := object.(*fyne.Container)
			title := box.Objects[0].(*widget.Label)
			subtitle := box.Objects[1].(*widget.Label)
			if strings.TrimSpace(s.Title) == "" {
				title.SetText("Session " + shortID(s.ID))
			} else {
				title.SetText(s.Title)
			}
			workspace := s.Workspace
			if strings.TrimSpace(workspace) == "" {
				workspace = "No workspace"
			}
			if s.Status != desktopstate.TaskIdle {
				workspace += " · " + string(s.Status)
			}
			subtitle.SetText(workspace)
		},
	)
	ui.list.OnSelected = func(id widget.ListItemID) {
		ui.mu.Lock()
		if id >= 0 && id < len(ui.state.Sessions) {
			selected := ui.state.Sessions[id].ID
			ui.state = desktopstate.Reduce(ui.state, desktopstate.Event{Kind: desktopstate.EventSessionSelected, SessionID: selected})
		}
		ui.mu.Unlock()
		ui.refreshActiveView()
	}

	search := widget.NewEntry()
	search.SetPlaceHolder("Search")
	newTask := widget.NewButtonWithIcon("", theme.ContentAddIcon(), ui.newSession)
	sidebarHeader := container.NewBorder(nil, nil, nil, newTask, widget.NewLabelWithStyle("protonMAN", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	sidebar := container.NewBorder(
		container.NewVBox(sidebarHeader, search),
		container.NewVBox(widget.NewSeparator(), widget.NewLabel("Desktop via ACP")),
		nil,
		nil,
		ui.list,
	)

	headerActions := container.NewHBox(ui.stop, ui.status)
	header := container.NewBorder(nil, nil, nil, headerActions,
		container.NewVBox(
			widget.NewLabelWithStyle("protonMAN", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewLabel("Coding agent · ACP"),
		),
	)
	composer := container.NewBorder(nil, nil, nil, ui.send, ui.composer)
	conversation := container.NewBorder(header, composer, nil, nil, container.NewVScroll(ui.chat))

	split := container.NewHSplit(sidebar, conversation)
	split.Offset = 0.29
	w.SetContent(container.NewPadded(split))

	go ui.connect()
	w.ShowAndRun()
	if ui.client != nil {
		_ = ui.client.Close()
	}
	return nil
}

func (a *application) connect() {
	binary := strings.TrimSpace(os.Getenv("PROTONMAN_BINARY"))
	if binary == "" {
		binary = "protonman"
	}
	client, err := acpclient.Start(a.ctx, binary, a.handleEvent)
	if err != nil {
		a.setStatus("Disconnected · " + err.Error())
		return
	}
	a.client = client

	var initResult struct {
		ProtocolVersion int `json:"protocolVersion"`
		AgentInfo       struct {
			Version string `json:"version"`
		} `json:"agentInfo"`
	}
	if err := client.Call(a.ctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo": map[string]any{
			"name": "protonman-desktop", "title": "Protonman Desktop",
		},
		"clientCapabilities": map[string]any{},
	}, &initResult); err != nil {
		a.setStatus("ACP initialize failed · " + err.Error())
		return
	}
	if initResult.ProtocolVersion != 1 {
		a.setStatus(fmt.Sprintf("Unsupported ACP v%d", initResult.ProtocolVersion))
		return
	}
	version := strings.TrimPrefix(initResult.AgentInfo.Version, "v")
	if version == "" {
		version = "connected"
	}
	a.setStatus("Protonman " + version + " · ACP v1")
	a.refreshSessions()
}

func (a *application) refreshSessions() {
	if a.client == nil {
		return
	}
	var result struct {
		Sessions []sessionItem `json:"sessions"`
	}
	if err := a.client.Call(a.ctx, "session/list", map[string]any{}, &result); err != nil {
		a.setStatus("Session list failed · " + err.Error())
		return
	}

	a.mu.Lock()
	old := make(map[string]desktopstate.SessionState, len(a.state.Sessions))
	for _, session := range a.state.Sessions {
		old[session.ID] = session
	}
	sessions := make([]desktopstate.SessionState, 0, len(result.Sessions))
	for _, session := range result.Sessions {
		projected := desktopstate.SessionState{ID: session.ID, Title: session.Title, Workspace: session.Cwd, Status: desktopstate.TaskIdle}
		if previous, ok := old[session.ID]; ok {
			projected.Status = previous.Status
			projected.Timeline = previous.Timeline
		}
		sessions = append(sessions, projected)
		if a.transcripts[session.ID] == nil {
			a.transcripts[session.ID] = &strings.Builder{}
		}
	}
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionsReplaced, Sessions: sessions})
	a.mu.Unlock()
	fyne.Do(func() { a.list.Refresh() })
	a.refreshActiveView()
}

func (a *application) newSession() {
	if a.client == nil {
		return
	}
	cwd, _ := os.Getwd()
	go func() {
		var result struct {
			SessionID string `json:"sessionId"`
		}
		if err := a.client.Call(a.ctx, "session/new", map[string]any{"cwd": cwd}, &result); err != nil {
			a.setStatus("New session failed · " + err.Error())
			return
		}
		a.refreshSessions()
		a.mu.Lock()
		a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionSelected, SessionID: result.SessionID})
		a.mu.Unlock()
		a.refreshActiveView()
	}()
}

func (a *application) sendPrompt() {
	text := strings.TrimSpace(a.composer.Text)
	if text == "" || a.client == nil {
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
	a.refreshActiveView()
	fyne.Do(func() { a.list.Refresh() })

	go func() {
		var result struct {
			StopReason string `json:"stopReason"`
		}
		err := a.client.Call(a.ctx, "session/prompt", map[string]any{
			"sessionId": sessionID,
			"prompt":    []map[string]any{{"type": "text", "text": text}},
		}, &result)

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
		fyne.Do(func() { a.list.Refresh() })
		a.refreshActiveView()
	}()
}

func (a *application) cancelPrompt() {
	if a.client == nil {
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
		if err := a.client.Call(a.ctx, "session/cancel", map[string]any{"sessionId": sessionID}, nil); err != nil {
			a.setStatus("Cancel failed · " + err.Error())
		}
	}()
}

func (a *application) handleEvent(event acpclient.Event) {
	if event.Method != "session/update" {
		return
	}
	var payload struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			Kind   string `json:"sessionUpdate"`
			Title  string `json:"title"`
			Status string `json:"status"`
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"update"`
	}
	if json.Unmarshal(event.Params, &payload) != nil {
		return
	}
	if payload.SessionID == "" {
		return
	}
	switch payload.Update.Kind {
	case "agent_message_chunk":
		a.appendTranscript(payload.SessionID, payload.Update.Content.Text)
	case "tool_call":
		a.appendTranscript(payload.SessionID, "\n\n`◐ "+payload.Update.Title+"`\n\n")
	case "tool_call_update":
		if payload.Update.Title != "" {
			a.appendTranscript(payload.SessionID, "\n`✓ "+payload.Update.Title+"`\n")
		}
	}
}

func (a *application) appendTranscript(sessionID, text string) {
	a.mu.Lock()
	builder := a.transcripts[sessionID]
	if builder == nil {
		builder = &strings.Builder{}
		a.transcripts[sessionID] = builder
	}
	builder.WriteString(text)
	active := a.state.ActiveSessionID == sessionID
	markdown := builder.String()
	a.mu.Unlock()
	if !active {
		return
	}
	fyne.Do(func() {
		a.chat.ParseMarkdown(markdown)
		a.chat.Refresh()
	})
}

func (a *application) refreshActiveView() {
	a.mu.Lock()
	activeID := a.state.ActiveSessionID
	busy := a.sessionBusyLocked(activeID)
	markdown := ""
	if transcript := a.transcripts[activeID]; transcript != nil {
		markdown = transcript.String()
	}
	a.mu.Unlock()
	fyne.Do(func() {
		a.chat.ParseMarkdown(markdown)
		a.chat.Refresh()
		if activeID == "" || busy {
			a.send.Disable()
		} else {
			a.send.Enable()
		}
		if busy {
			a.stop.Enable()
		} else {
			a.stop.Disable()
		}
	})
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
