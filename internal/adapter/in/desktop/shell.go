//go:build desktop

package desktop

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	sidebarTitleMaxRunes = 38
	sidebarMetaMaxRunes  = 28
	contextDrawerWidth   = 320
)

func (a *application) initDesktopControls() {
	a.status = widget.NewLabel("Connecting to Protonman…")
	a.status.Wrapping = fyne.TextWrapWord

	a.chat = widget.NewRichTextFromMarkdown("")
	a.chat.Wrapping = fyne.TextWrapWord
	a.composer = widget.NewMultiLineEntry()
	a.composer.SetPlaceHolder("Ask protonMAN…")
	a.composer.Wrapping = fyne.TextWrapWord
	a.send = widget.NewButton("Send", a.sendPrompt)
	a.stop = widget.NewButtonWithIcon("", theme.MediaStopIcon(), a.cancelPrompt)
	a.send.Disable()
	a.stop.Disable()

	a.initPermissionControls()
	a.initRuntimeControls()
	a.initInspectorControls()
	a.initIntegrationControls()
	a.initSessionList()
}

func (a *application) initPermissionControls() {
	a.permissionInbox = widget.NewButton("Permissions 0", a.selectNextPermission)
	a.permissionTitle = widget.NewLabelWithStyle("Permission required", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	a.permissionDetail = widget.NewLabel("")
	a.permissionDetail.Wrapping = fyne.TextWrapWord
	a.permissionActions = container.NewHBox()
	a.permissionPanel = container.NewVBox(
		widget.NewSeparator(),
		a.permissionTitle,
		a.permissionDetail,
		a.permissionActions,
		widget.NewSeparator(),
	)
	a.permissionPanel.Hide()
}

func (a *application) initRuntimeControls() {
	a.modelProvider = widget.NewEntry()
	a.modelProvider.SetPlaceHolder("provider")
	a.modelID = widget.NewEntry()
	a.modelID.SetPlaceHolder("model")
	a.applyModel = widget.NewButton("Apply model", a.setRuntimeModel)
	a.reasoningSelect = widget.NewSelect([]string{"auto", "none", "minimal", "low", "medium", "high", "xhigh", "max"}, func(value string) {
		if !a.runtimeSync {
			a.setRuntimeReasoning(value)
		}
	})
	a.lowSelect = widget.NewSelect([]string{"auto", "on", "off"}, func(value string) {
		if !a.runtimeSync {
			a.setRuntimeLowConcurrency(value)
		}
	})

	a.runtimeSummary = widget.NewButton("Model", a.toggleRuntimePanel)
	a.runtimePanel = container.NewVBox(
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Runtime", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewGridWithColumns(2, widget.NewLabel("Provider"), a.modelProvider),
		container.NewGridWithColumns(2, widget.NewLabel("Model"), a.modelID),
		a.applyModel,
		container.NewGridWithColumns(2, widget.NewLabel("Reasoning"), a.reasoningSelect),
		container.NewGridWithColumns(2, widget.NewLabel("Low concurrency"), a.lowSelect),
	)
	a.runtimePanel.Hide()
}

func (a *application) initInspectorControls() {
	a.contextContent = widget.NewRichTextFromMarkdown("_No session context loaded._")
	a.contextContent.Wrapping = fyne.TextWrapWord
	a.contextToggle = widget.NewButton("Context", a.toggleContextDrawer)
	closeButton := widget.NewButton("Close", a.toggleContextDrawer)
	drawerHeader := container.NewBorder(nil, nil, nil, closeButton,
		widget.NewLabelWithStyle("Context", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)
	drawer := container.NewBorder(
		drawerHeader,
		nil,
		widget.NewSeparator(),
		nil,
		container.NewVScroll(a.contextContent),
	)
	a.contextDrawer = container.New(fixedWidthLayout{width: contextDrawerWidth}, drawer)
	a.contextDrawer.Hide()
}

func (a *application) initSessionList() {
	a.list = widget.NewList(
		func() int {
			a.mu.Lock()
			defer a.mu.Unlock()
			return len(a.sidebarRows)
		},
		func() fyne.CanvasObject {
			title := newNerdIconText(iconSession, "Session", fyne.TextStyle{}, false)
			subtitle := newNerdIconText(iconFolder, "workspace", fyne.TextStyle{}, true)
			return container.NewVBox(title, subtitle)
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			a.bindSessionRow(id, object)
		},
	)
	a.list.OnSelected = a.selectSessionRow
}

func (a *application) bindSessionRow(id widget.ListItemID, object fyne.CanvasObject) {
	a.mu.Lock()
	if id < 0 || id >= len(a.sidebarRows) {
		a.mu.Unlock()
		return
	}
	row := a.sidebarRows[id]
	var session desktopstate.SessionState
	if row.Kind == sidebarSessionRow {
		for _, candidate := range a.state.Sessions {
			if candidate.ID == row.SessionID {
				session = candidate
				break
			}
		}
	}
	a.mu.Unlock()

	box := object.(*fyne.Container)
	title := box.Objects[0].(*fyne.Container)
	subtitle := box.Objects[1].(*fyne.Container)
	if row.Kind == sidebarWorkspaceRow {
		setNerdIconText(title, iconFolder, compactText(row.WorkspaceName, sidebarTitleMaxRunes))
		setNerdIconText(subtitle, iconSession, fmt.Sprintf("%d sessions", row.SessionCount))
		return
	}

	titleText := strings.TrimSpace(session.Title)
	if titleText == "" {
		titleText = "Session " + shortID(session.ID)
	}
	setNerdIconText(title, iconSession, compactText(titleText, sidebarTitleMaxRunes))

	if session.Status != desktopstate.TaskIdle {
		setNerdIconText(subtitle, taskStatusIcon(session.Status), string(session.Status))
		return
	}
	workspace := strings.TrimSpace(session.WorkspaceName)
	if workspace == "" {
		workspace = "workspace"
	}
	setNerdIconText(subtitle, iconFolder, compactText(workspace, sidebarMetaMaxRunes))
}

func (a *application) selectSessionRow(id widget.ListItemID) {
	a.mu.Lock()
	if id < 0 || id >= len(a.sidebarRows) {
		a.mu.Unlock()
		return
	}
	row := a.sidebarRows[id]
	if row.Kind != sidebarSessionRow {
		activeIndex := sidebarRowIndexForSession(a.sidebarRows, a.state.ActiveSessionID)
		a.mu.Unlock()
		if activeIndex >= 0 {
			fyne.Do(func() { a.list.Select(widget.ListItemID(activeIndex)) })
		}
		return
	}
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionSelected, SessionID: row.SessionID})
	a.mu.Unlock()
	a.refreshActiveView()
	a.refreshPermissionView()
}

func (a *application) buildDesktopShell() fyne.CanvasObject {
	sidebar := a.buildSidebar()
	conversation := a.buildConversationSurface()

	split := container.NewHSplit(sidebar, conversation)
	split.Offset = 0.22
	return container.NewPadded(split)
}

func (a *application) buildSidebar() fyne.CanvasObject {
	search := widget.NewEntry()
	search.SetPlaceHolder("Search")
	newTask := widget.NewButtonWithIcon("", theme.ContentAddIcon(), a.newSession)
	sidebarHeader := container.NewBorder(nil, nil, nil, newTask,
		newNerdIconText(iconRocket, "protonMAN", fyne.TextStyle{Bold: true}, false),
	)

	secondary := container.NewVBox(
		widget.NewSeparator(),
		container.NewGridWithColumns(2, a.integrationButton, a.permissionInbox),
		a.integrationPanel,
	)
	return container.NewBorder(
		container.NewVBox(sidebarHeader, search),
		secondary,
		nil,
		nil,
		a.list,
	)
}

func (a *application) buildConversationSurface() fyne.CanvasObject {
	a.sessionTitle = widget.NewLabelWithStyle("protonMAN", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	a.sessionMeta = widget.NewLabel("Select a session")
	a.sessionMeta.Wrapping = fyne.TextWrapWord

	headerActions := container.NewHBox(a.stop, a.runtimeSummary, a.contextToggle)
	header := container.NewBorder(nil, nil, nil, headerActions,
		container.NewVBox(a.sessionTitle, a.sessionMeta),
	)

	conversationScroll := container.NewVScroll(a.chat)
	conversationBody := container.NewBorder(a.permissionPanel, nil, nil, a.contextDrawer, conversationScroll)

	composer := container.NewBorder(nil, nil, nil, a.send, a.composer)
	footer := container.NewVBox(composer, a.status)

	return container.NewBorder(
		container.NewVBox(header, a.runtimePanel, widget.NewSeparator()),
		footer,
		nil,
		nil,
		conversationBody,
	)
}

func (a *application) toggleRuntimePanel() {
	if a.runtimePanel.Visible() {
		a.runtimePanel.Hide()
		return
	}
	a.runtimePanel.Show()
}

func (a *application) toggleContextDrawer() {
	if a.contextDrawer.Visible() {
		a.contextDrawer.Hide()
		return
	}
	a.contextDrawer.Show()
}
