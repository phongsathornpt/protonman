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
)

type sessionItem struct {
	ID    string `json:"sessionId"`
	Title string `json:"title"`
	Cwd   string `json:"cwd"`
}

type application struct {
	ctx context.Context

	client *acpclient.Client

	mu        sync.Mutex
	sessions  []sessionItem
	activeID  string
	transcript strings.Builder

	status    *widget.Label
	list      *widget.List
	chat      *widget.RichText
	composer  *widget.Entry
	send      *widget.Button
}

// Run starts Protonman Desktop. The desktop is deliberately a thin ACP client;
// the Protonman CLI remains the single runtime for sessions, tools and models.
func Run(ctx context.Context) error {
	a := app.NewWithID("ai.protonman.desktop")
	a.Settings().SetTheme(theme.DarkTheme())
	w := a.NewWindow("Protonman")
	w.Resize(fyne.NewSize(1220, 780))

	ui := &application{ctx: ctx}
	ui.status = widget.NewLabel("Connecting to Protonman…")
	ui.chat = widget.NewRichTextFromMarkdown("")
	ui.composer = widget.NewEntry()
	ui.composer.SetPlaceHolder("Message protonMAN…")
	ui.send = widget.NewButton("Send", ui.sendPrompt)
	ui.send.Disable()

	ui.list = widget.NewList(
		func() int {
			ui.mu.Lock()
			defer ui.mu.Unlock()
			return len(ui.sessions)
		},
		func() fyne.CanvasObject {
			title := widget.NewLabel("Session")
			subtitle := widget.NewLabel("workspace")
			subtitle.Importance = widget.LowImportance
			return container.NewVBox(title, subtitle)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			ui.mu.Lock()
			if id < 0 || id >= len(ui.sessions) {
				ui.mu.Unlock()
				return
			}
			s := ui.sessions[id]
			ui.mu.Unlock()
			box := object.(*fyne.Container)
			title := box.Objects[0].(*widget.Label)
			subtitle := box.Objects[1].(*widget.Label)
			if strings.TrimSpace(s.Title) == "" {
				title.SetText("Session " + shortID(s.ID))
			} else {
				title.SetText(s.Title)
			}
			if strings.TrimSpace(s.Cwd) == "" {
				subtitle.SetText("No workspace")
			} else {
				subtitle.SetText(s.Cwd)
			}
		},
	)
	ui.list.OnSelected = func(id widget.ListItemID) {
		ui.mu.Lock()
		if id >= 0 && id < len(ui.sessions) {
			ui.activeID = ui.sessions[id].ID
		}
		active := ui.activeID
		ui.mu.Unlock()
		if active != "" {
			ui.send.Enable()
		}
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

	header := container.NewBorder(nil, nil, nil, ui.status,
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
		AgentInfo struct {
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
	a.sessions = result.Sessions
	a.mu.Unlock()
	fyne.Do(func() { a.list.Refresh() })
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
		a.mu.Lock()
		a.activeID = result.SessionID
		a.mu.Unlock()
		a.refreshSessions()
		fyne.Do(func() { a.send.Enable() })
	}()
}

func (a *application) sendPrompt() {
	text := strings.TrimSpace(a.composer.Text)
	if text == "" || a.client == nil {
		return
	}
	a.mu.Lock()
	sessionID := a.activeID
	a.mu.Unlock()
	if sessionID == "" {
		return
	}
	a.composer.SetText("")
	a.appendTranscript("\n\n> " + text + "\n\n")
	a.send.Disable()
	go func() {
		var result struct {
			StopReason string `json:"stopReason"`
		}
		err := a.client.Call(a.ctx, "session/prompt", map[string]any{
			"sessionId": sessionID,
			"prompt": []map[string]any{{"type": "text", "text": text}},
		}, &result)
		if err != nil {
			a.appendTranscript("\n\n**Error:** " + err.Error() + "\n")
		}
		fyne.Do(func() { a.send.Enable() })
	}()
}

func (a *application) handleEvent(event acpclient.Event) {
	if event.Method != "session/update" {
		return
	}
	var payload struct {
		Update struct {
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
	switch payload.Update.Kind {
	case "agent_message_chunk":
		a.appendTranscript(payload.Update.Content.Text)
	case "tool_call":
		a.appendTranscript("\n\n`◐ " + payload.Update.Title + "`\n\n")
	case "tool_call_update":
		if payload.Update.Title != "" {
			a.appendTranscript("\n`✓ " + payload.Update.Title + "`\n")
		}
	}
}

func (a *application) appendTranscript(text string) {
	a.mu.Lock()
	a.transcript.WriteString(text)
	markdown := a.transcript.String()
	a.mu.Unlock()
	fyne.Do(func() {
		a.chat.ParseMarkdown(markdown)
		a.chat.Refresh()
	})
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
