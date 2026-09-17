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
	a.status = widget.NewLabel("Connecting to ACP agents…")
	a.status.Wrapping = fyne.TextWrapWord
	a.status.Importance = widget.LowImportance

	a.chat = widget.NewRichTextFromMarkdown("")
	a.chat.Wrapping = fyne.TextWrapWord
	a.composer = widget.NewMultiLineEntry()
	a.composer.SetPlaceHolder("Ask the agent… · Shift+Enter to send")
	a.composer.SetMinRowsVisible(3)
	a.composer.Wrapping = fyne.TextWrapWord
	a.composer.OnSubmitted = func(_ string) { a.submitPrompt() }
	a.send = widget.NewButton("Send", a.submitPrompt)
	a.stop = widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), a.cancelPrompt)
	a.send.Disable()
	a.stop.Disable()
	a.sessionAgentBadge = widget.NewButton("● Agent", a.onSessionAgentBadgeClicked)
	a.sessionAgentBadge.Importance = widget.LowImportance
	a.sessionAgentBadge.Hide()

	a.initPermissionControls()
	a.initACPAgentControls()
	a.initRuntimeControls()
	a.initInspectorControls()
	a.initIntegrationControls()
	a.integrationButton.OnTapped = a.toggleIntegrationPanel
	a.initSessionList()
}

func (a *application) initPermissionControls() {
	a.permissionInbox = widget.NewButton("Permissions 0", a.selectNextPermission)
	a.permissionInbox.Disable()
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
		widget.NewLabelWithStyle("Runtime controls", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
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
		fixedWidthLayout{width: contextDrawerPreferredWidth},
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
			title := newIconText(iconSession, "Session", fyne.TextStyle{}, false)
			subtitle := newIconText(iconFolder, "workspace", fyne.TextStyle{}, true)
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
	collapsed := false
	activeSessionID := a.state.ActiveSessionID
	var session desktopstate.SessionState
	if row.Kind == sidebarSessionRow {
		for _, candidate := range a.state.Sessions {
			if candidate.ID == row.SessionID {
				session = candidate
				break
			}
		}
	} else {
		collapsed = a.collapsedWorkspaces[row.WorkspaceKey]
	}
	a.mu.Unlock()

	box := object.(*fyne.Container)
	title := box.Objects[0].(*fyne.Container)
	subtitle := box.Objects[1].(*fyne.Container)
	if row.Kind == sidebarWorkspaceRow {
		workspaceIcon := iconFolder
		if collapsed {
			workspaceIcon = iconCollapsed
		}
		setIconText(title, workspaceIcon, compactText(row.WorkspaceName, sidebarTitleMaxRunes))
		setIconText(subtitle, iconSession, fmt.Sprintf("%d sessions", row.SessionCount))
		setImportance(subtitle, widget.LowImportance)
		if label := titleLabel(title); label != nil {
			label.TextStyle = fyne.TextStyle{Bold: true}
			label.Refresh()
		}
		return
	}

	titleText := strings.TrimSpace(session.Title)
	if titleText == "" {
		titleText = "Session " + shortID(session.ID)
	}
	setIconText(title, iconSession, compactText(titleText, sidebarTitleMaxRunes))
	if label := titleLabel(title); label != nil {
		if activeSessionID == session.ID {
			label.TextStyle = fyne.TextStyle{Bold: true}
		} else {
			label.TextStyle = fyne.TextStyle{}
		}
		label.Refresh()
	}

	if session.Status != desktopstate.TaskIdle {
		statusText := string(session.Status)
		if agent := a.agentNameFor(session.AgentID); agent != "" {
			statusText += " · " + agent
		}
		setIconText(subtitle, taskStatusIcon(session.Status), compactText(statusText, sidebarMetaMaxRunes))
		setImportance(subtitle, statusImportance(session.Status))
		return
	}
	workspace := strings.TrimSpace(session.WorkspaceName)
	if workspace == "" {
		workspace = "workspace"
	}
	if agent := a.agentNameFor(session.AgentID); agent != "" {
		workspace += " · " + agent
	}
	setIconText(subtitle, iconFolder, compactText(workspace, sidebarMetaMaxRunes))
	setImportance(subtitle, widget.LowImportance)
}

func (a *application) selectSessionRow(id widget.ListItemID) {
	a.mu.Lock()
	if id < 0 || id >= len(a.sidebarRows) {
		a.mu.Unlock()
		return
	}
	row := a.sidebarRows[id]
	if row.Kind != sidebarSessionRow {
		a.collapsedWorkspaces[row.WorkspaceKey] = !a.collapsedWorkspaces[row.WorkspaceKey]
		a.rebuildSidebarRowsLocked()
		activeIndex := sidebarRowIndexForSession(a.sidebarRows, a.state.ActiveSessionID)
		a.mu.Unlock()
		a.list.Refresh()
		a.refreshSidebarEmptyState()
		if activeIndex >= 0 {
			a.list.Select(widget.ListItemID(activeIndex))
		} else {
			a.list.UnselectAll()
		}
		return
	}
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionSelected, SessionID: row.SessionID})
	a.mu.Unlock()
	a.loadSessionHistory(row.SessionID)
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
	a.refreshSidebarEmptyState()
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
	a.sessionSearch.SetPlaceHolder("Search sessions or workspaces")
	a.sessionSearch.OnChanged = a.applySidebarQuery
	title := newIconText(iconRocket, "Protonman Desktop", fyne.TextStyle{Bold: true}, false)
	newTask := widget.NewButtonWithIcon("New session", theme.ContentAddIcon(), func() {
		if a.sessionSearch.Text != "" {
			a.sessionSearch.SetText("")
		}
		a.newSession()
	})
	newTask.Importance = widget.HighImportance
	agentPicker := container.NewBorder(nil, nil,
		widget.NewLabel("New with"),
		nil,
		a.agentSelect,
	)

	secondary := container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(a.agentSettingsButton, a.integrationButton, a.permissionInbox),
	)
	a.sidebarEmpty = widget.NewLabel("")
	a.sidebarEmpty.Alignment = fyne.TextAlignCenter
	a.sidebarEmpty.Wrapping = fyne.TextWrapWord
	a.sidebarEmpty.Importance = widget.LowImportance
	a.sidebarEmpty.Hide()
	listContent := container.NewMax(a.list, a.sidebarEmpty)

	return container.NewBorder(
		container.NewVBox(title, newTask, agentPicker, a.sessionSearch),
		secondary,
		nil,
		nil,
		listContent,
	)
}

func (a *application) refreshSidebarEmptyState() {
	if a.sidebarEmpty == nil {
		return
	}
	a.mu.Lock()
	empty := len(a.sidebarRows) == 0
	query := a.sidebarQuery != ""
	a.mu.Unlock()
	if !empty {
		a.sidebarEmpty.Hide()
		return
	}
	if query {
		a.sidebarEmpty.SetText("No matching sessions")
	} else {
		a.sidebarEmpty.SetText("No sessions yet\nStart a new session to begin")
	}
	a.sidebarEmpty.Show()
}

func (a *application) buildConversationSurface() fyne.CanvasObject {
	a.sessionTitle = widget.NewLabelWithStyle("protonMAN", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	a.sessionMeta = widget.NewLabel("Select a session")
	a.sessionMeta.Wrapping = fyne.TextWrapWord

	stopControl := container.New(fixedWidthLayout{width: 76}, a.stop)
	headerActions := container.NewHBox(a.sessionAgentBadge, stopControl, a.runtimeSummary, a.contextToggle)
	header := container.NewBorder(nil, nil, nil, headerActions,
		container.NewVBox(a.sessionTitle, a.sessionMeta),
	)

	scroll := container.NewVScroll(a.chat)
	a.conversationScroll = scroll
	conversationWithDrawer := container.New(responsiveDrawerLayout{}, scroll, a.contextDrawer)
	conversationBody := container.NewMax(conversationWithDrawer)
	auxiliaryContent := container.NewVBox(a.permissionPanel, a.agentSettingsPanel, a.runtimePanel, a.integrationPanel, widget.NewSeparator())
	auxiliaryScroll := container.NewVScroll(auxiliaryContent)

	composer := container.NewBorder(nil, nil, nil, a.send, a.composer)
	footer := container.NewVBox(composer, a.status)

	return container.New(desktopSurfaceLayout{}, header, auxiliaryScroll, footer, conversationBody)
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
	a.contextDrawer.Hide()
	a.integrationPanel.Hide()
	a.runtimePanel.Show()
}

func (a *application) toggleContextDrawer() {
	if a.contextDrawer.Visible() {
		a.contextDrawer.Hide()
		return
	}
	a.runtimePanel.Hide()
	a.integrationPanel.Hide()
	a.contextDrawer.Show()
}

func (a *application) toggleIntegrationPanel() {
	if a.integrationPanel.Visible() {
		a.integrationPanel.Hide()
		return
	}
	a.runtimePanel.Hide()
	a.contextDrawer.Hide()
	a.integrationPanel.Show()
}
