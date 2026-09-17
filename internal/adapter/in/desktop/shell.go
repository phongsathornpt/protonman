//go:build desktop

package desktop

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
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
	activeSessionID := a.state.ActiveSessionID
	var session desktopstate.SessionState
	if row.Kind == sidebarSessionRow {
		for _, candidate := range a.state.Sessions {
			if candidate.ID == row.SessionID {
				session = candidate
				break
			}
		}
	}
	var project desktopstate.ProjectState
	var projectOK bool
	if row.Kind == sidebarProjectRow {
		project, projectOK = a.projectByIDLocked(row.ProjectID)
	}
	a.mu.Unlock()

	box := object.(*fyne.Container)
	title := box.Objects[0].(*fyne.Container)
	subtitle := box.Objects[1].(*fyne.Container)
	if row.Kind == sidebarProjectRow {
		if !projectOK {
			return
		}
		setIconText(title, iconFolder, compactText(project.Name, sidebarTitleMaxRunes))
		sessions := 0
		running := 0
		for _, candidate := range a.state.Sessions {
			if candidate.ProjectID == project.ID {
				sessions++
				if candidate.Status != desktopstate.TaskIdle && candidate.Status != desktopstate.TaskCompleted && candidate.Status != desktopstate.TaskFailed {
					running++
				}
			}
		}
		meta := fmt.Sprintf("%d folders · %d agents · %d conversations", len(project.Folders), len(project.AgentIDs), sessions)
		if running > 0 {
			meta += fmt.Sprintf(" · %d active", running)
		}
		setIconText(subtitle, iconSession, compactText(meta, sidebarMetaMaxRunes+20))
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
		setIconText(subtitle, taskStatusIcon(session.Status), string(session.Status))
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
	if row.Kind == sidebarProjectRow {
		a.state.ActiveProjectID = row.ProjectID
		a.state.ActiveSessionID = ""
		project, _ := a.projectByIDLocked(row.ProjectID)
		agentID := a.agentForProjectLocked(project)
		a.mu.Unlock()
		if agentID != "" {
			a.agentSelect.SetSelected(agentID)
		}
		a.refreshActiveView()
		a.refreshPermissionView()
		return
	}
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionSelected, SessionID: row.SessionID})
	selectedAgent := ""
	for _, session := range a.state.Sessions {
		if session.ID == row.SessionID {
			a.state.ActiveProjectID = session.ProjectID
			if session.AgentID != "" {
				a.activeAgentID = session.AgentID
				selectedAgent = session.AgentID
			}
			break
		}
	}
	a.mu.Unlock()
	if selectedAgent != "" && a.agentSelect != nil {
		a.agentSelect.SetSelected(selectedAgent)
	}
	a.loadSessionHistory(row.SessionID)
	a.refreshActiveView()
	a.refreshPermissionView()
}

func (a *application) applySidebarQuery(query string) {
	a.mu.Lock()
	a.sidebarQuery = strings.TrimSpace(query)
	a.rebuildSidebarRowsLocked()
	activeIndex := sidebarRowIndexForProject(a.sidebarRows, a.state.ActiveProjectID)
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
	a.sessionSearch.SetPlaceHolder("Search projects, folders, or conversations")
	a.sessionSearch.OnChanged = a.applySidebarQuery
	newProject := widget.NewButtonWithIcon("Project", theme.ContentAddIcon(), func() {
		a.createProjectFromCurrentFolder()
	})
	addFolder := widget.NewButtonWithIcon("Folder", theme.FolderOpenIcon(), func() {
		if a.window == nil {
			return
		}
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			a.addFolderToActiveProject(uri.Path())
		}, a.window)
	})
	newTask := widget.NewButtonWithIcon("Chat", theme.MailSendIcon(), func() {
		if a.sessionSearch.Text != "" {
			a.sessionSearch.SetText("")
		}
		a.newSession()
	})
	sidebarHeader := container.NewBorder(nil, nil, nil, container.NewHBox(newProject, addFolder, newTask),
		newIconText(iconRocket, "Protonman Desktop", fyne.TextStyle{Bold: true}, false),
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
		container.NewVBox(sidebarHeader, a.sessionSearch),
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
		a.sidebarEmpty.SetText("No matching projects")
	} else {
		a.sidebarEmpty.SetText("No projects yet\nAdd a project folder to begin")
	}
	a.sidebarEmpty.Show()
}

func (a *application) buildConversationSurface() fyne.CanvasObject {
	a.sessionTitle = widget.NewLabelWithStyle("protonMAN", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	a.sessionMeta = widget.NewLabel("Select a session")
	a.sessionMeta.Wrapping = fyne.TextWrapWord
	a.projectTitle = widget.NewLabelWithStyle("Project", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	a.projectFolders = widget.NewLabel("")
	a.projectFolders.Wrapping = fyne.TextWrapWord
	a.projectAgents = widget.NewLabel("")
	a.projectAgents.Wrapping = fyne.TextWrapWord
	a.projectRecent = container.NewVBox()
	a.projectOverview = container.NewVBox(
		a.projectTitle,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Folders", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		a.projectFolders,
		widget.NewLabelWithStyle("ACP agents", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		a.projectAgents,
		widget.NewLabelWithStyle("Recent conversations", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		a.projectRecent,
	)
	a.projectOverview.Hide()

	agentLabel := widget.NewLabelWithStyle("Agent", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	agentControl := container.New(fixedWidthLayout{width: 150}, a.agentSelect)
	stopControl := container.New(fixedWidthLayout{width: 76}, a.stop)
	headerActions := container.NewHBox(agentLabel, agentControl, stopControl, a.runtimeSummary, a.contextToggle)
	header := container.NewBorder(nil, nil, nil, headerActions,
		container.NewVBox(a.sessionTitle, a.sessionMeta),
	)

	scroll := container.NewVScroll(a.chat)
	a.conversationScroll = scroll
	conversationWithDrawer := container.New(responsiveDrawerLayout{}, scroll, a.contextDrawer)
	conversationBody := container.NewMax(conversationWithDrawer, a.projectOverview)
	auxiliaryContent := container.NewVBox(a.permissionPanel, a.agentSettingsPanel, a.runtimePanel, a.integrationPanel, widget.NewSeparator())
	auxiliaryScroll := container.NewVScroll(auxiliaryContent)

	composer := container.NewBorder(nil, nil, nil, a.send, a.composer)
	footer := container.NewVBox(composer, a.status)

	return container.New(desktopSurfaceLayout{}, header, auxiliaryScroll, footer, conversationBody)
}

func (a *application) renderProjectOverview() {
	if a.projectOverview == nil {
		return
	}
	a.mu.Lock()
	project, ok := a.projectByIDLocked(a.state.ActiveProjectID)
	if !ok || a.state.ActiveSessionID != "" {
		a.mu.Unlock()
		fyne.Do(func() { a.projectOverview.Hide() })
		return
	}
	sessions := make([]desktopstate.SessionState, 0)
	for _, session := range a.state.Sessions {
		if session.ProjectID == project.ID {
			sessions = append(sessions, session)
		}
	}
	agents := make([]string, 0, len(project.AgentIDs))
	for _, agentID := range project.AgentIDs {
		name := agentID
		if profile, exists := a.profiles[agentID]; exists {
			name = profile.DisplayName
		}
		if name == "" {
			name = agentID
		}
		agents = append(agents, name)
	}
	projectName := project.Name
	folders := projectFolderNames(project)
	agentText := strings.Join(agents, ", ")
	if agentText == "" {
		agentText = "No ACP agents attached"
	}
	type recentSession struct{ id, title, meta string }
	recent := make([]recentSession, 0, len(sessions))
	for i := len(sessions) - 1; i >= 0 && len(recent) < 8; i-- {
		session := sessions[i]
		title := strings.TrimSpace(session.Title)
		if title == "" {
			title = "Session " + shortID(session.ID)
		}
		agentName := session.AgentID
		if profile, exists := a.profiles[session.AgentID]; exists {
			agentName = profile.DisplayName
		}
		recent = append(recent, recentSession{id: session.ID, title: title, meta: agentName + " · " + session.WorkspaceName})
	}
	a.mu.Unlock()

	fyne.Do(func() {
		a.projectTitle.SetText(projectName)
		a.projectFolders.SetText(folders)
		a.projectAgents.SetText(agentText)
		a.projectRecent.Objects = nil
		if len(recent) == 0 {
			a.projectRecent.Add(widget.NewLabel("No conversations yet. Start a chat from this project."))
		} else {
			for _, item := range recent {
				item := item
				a.projectRecent.Add(widget.NewButton(item.title+"\n"+item.meta, func() { a.selectProjectSession(item.id) }))
			}
		}
		a.projectRecent.Refresh()
		a.projectOverview.Show()
	})
}

func (a *application) selectProjectSession(sessionID string) {
	a.mu.Lock()
	if !desktopSessionExists(a.state.Sessions, sessionID) {
		a.mu.Unlock()
		return
	}
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionSelected, SessionID: sessionID})
	for _, session := range a.state.Sessions {
		if session.ID == sessionID {
			a.state.ActiveProjectID = session.ProjectID
			break
		}
	}
	a.mu.Unlock()
	a.loadSessionHistory(sessionID)
	a.refreshActiveView()
	a.refreshPermissionView()
}

func desktopSessionExists(sessions []desktopstate.SessionState, id string) bool {
	for _, session := range sessions {
		if session.ID == id {
			return true
		}
	}
	return false
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
