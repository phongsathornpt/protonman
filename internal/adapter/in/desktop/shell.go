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
	sidebarTitleMaxRunes        = 38
	sidebarMetaMaxRunes         = 28
	contextDrawerPreferredWidth = 320
	contextDrawerMinWidth       = 220
	contextDrawerWindowRatio    = 0.30
	conversationTailThreshold   = 64
)

func (a *application) initDesktopControls() {
	a.status = widget.NewLabel("Connecting to Protonman…")
	a.status.Wrapping = fyne.TextWrapWord

	a.chat = widget.NewRichTextFromMarkdown("")
	a.chat.Wrapping = fyne.TextWrapWord
	a.composer = widget.NewMultiLineEntry()
	a.composer.SetPlaceHolder("Ask protonMAN… · Shift+Enter to send")
	a.composer.SetMinRowsVisible(3)
	a.composer.Wrapping = fyne.TextWrapWord
	a.composer.OnSubmitted = func(_ string) { a.sendPrompt() }
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
	a.contextDrawer = container.New(
		fixedWidthLayout{width: contextDrawerWidthFor(a.desktopWindowWidth())},
		drawer,
	)
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

func (a *application) applySidebarQuery(query string) {
	a.mu.Lock()
	a.sidebarQuery = strings.TrimSpace(query)
	a.rebuildSidebarRowsLocked()
	activeIndex := sidebarRowIndexForSession(a.sidebarRows, a.state.ActiveSessionID)
	a.mu.Unlock()

	a.list.Refresh()
	if activeIndex >= 0 {
		a.list.Select(widget.ListItemID(activeIndex))
		return
	}
	a.list.UnselectAll()
}

func (a *application) buildDesktopShell() fyne.CanvasObject {
	sidebar := a.buildSidebar()
	conversation := a.buildConversationSurface()

	split := container.NewHSplit(sidebar, conversation)
	split.Offset = 0.22
	return container.NewPadded(split)
}

func (a *application) buildSidebar() fyne.CanvasObject {
	a.sessionSearch = widget.NewEntry()
	a.sessionSearch.SetPlaceHolder("Search sessions")
	a.sessionSearch.OnChanged = a.applySidebarQuery
	newTask := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() {
		if a.sessionSearch.Text != "" {
			a.sessionSearch.SetText("")
		}
		a.newSession()
	})
	sidebarHeader := container.NewBorder(nil, nil, nil, newTask,
		newNerdIconText(iconRocket, "protonMAN", fyne.TextStyle{Bold: true}, false),
	)

	secondary := container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(a.integrationButton, a.permissionInbox),
		a.integrationPanel,
	)
	return container.NewBorder(
		container.NewVBox(sidebarHeader, a.sessionSearch),
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

	scroll := container.NewVScroll(a.chat)
	a.conversationScroll = scroll
	conversationBody := container.NewBorder(a.permissionPanel, nil, nil, a.contextDrawer, scroll)

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

func (a *application) shouldFollowConversationTail() bool {
	scroll, ok := a.conversationScroll.(*container.Scroll)
	if !ok || scroll == nil || scroll.Content == nil {
		return true
	}
	viewportHeight := scroll.Size().Height
	contentHeight := scroll.Content.MinSize().Height
	if viewportHeight <= 0 || contentHeight <= viewportHeight {
		return true
	}
	maxOffset := contentHeight - viewportHeight
	return maxOffset-scroll.Offset.Y <= conversationTailThreshold
}

func (a *application) scrollConversationToBottom() {
	scroll, ok := a.conversationScroll.(*container.Scroll)
	if !ok || scroll == nil || scroll.Content == nil {
		return
	}
	scroll.ScrollToBottom()
}

func (a *application) desktopWindowWidth() float32 {
	if a.desktopApp == nil || a.desktopApp.Driver() == nil {
		return 0
	}
	windows := a.desktopApp.Driver().AllWindows()
	if len(windows) == 0 || windows[0] == nil || windows[0].Canvas() == nil {
		return 0
	}
	return windows[0].Canvas().Size().Width
}

func contextDrawerWidthFor(windowWidth float32) float32 {
	if windowWidth <= 0 {
		return contextDrawerPreferredWidth
	}
	width := windowWidth * contextDrawerWindowRatio
	if width < contextDrawerMinWidth {
		return contextDrawerMinWidth
	}
	if width > contextDrawerPreferredWidth {
		return contextDrawerPreferredWidth
	}
	return width
}

func (a *application) toggleRuntimePanel() {
	if a.runtimePanel.Visible() {
		a.runtimePanel.Hide()
		return
	}
	if a.contextDrawer.Visible() {
		a.contextDrawer.Hide()
	}
	a.runtimePanel.Show()
}

func (a *application) toggleContextDrawer() {
	if a.contextDrawer.Visible() {
		a.contextDrawer.Hide()
		return
	}
	if a.runtimePanel.Visible() {
		a.runtimePanel.Hide()
	}
	a.contextDrawer.Layout = fixedWidthLayout{width: contextDrawerWidthFor(a.desktopWindowWidth())}
	a.contextDrawer.Refresh()
	a.contextDrawer.Show()
}
