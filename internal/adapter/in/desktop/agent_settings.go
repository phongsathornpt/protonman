//go:build desktop

package desktop

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func (a *application) initACPAgentControls() {
	a.agentSettingsButton = widget.NewButton("ACP agents", a.toggleACPAgentPanel)
	a.agentSettingsSummary = widget.NewLabel("")
	a.agentSettingsSummary.Wrapping = fyne.TextWrapWord
	a.agentIDEntry = widget.NewEntry()
	a.agentIDEntry.SetPlaceHolder("id, e.g. antigravity")
	a.agentNameEntry = widget.NewEntry()
	a.agentNameEntry.SetPlaceHolder("display name")
	a.agentCommandEntry = widget.NewEntry()
	a.agentCommandEntry.SetPlaceHolder("ACP executable path")
	a.agentArgsEntry = widget.NewEntry()
	a.agentArgsEntry.SetPlaceHolder(`arguments JSON, e.g. ["--uid=desktop"]`)
	save := widget.NewButton("Save agent", a.saveACPAgent)
	restart := widget.NewButton("Restart agent", a.restartCurrentEditorAgent)
	remove := widget.NewButton("Remove agent", a.removeACPAgent)

	a.agentScanSelect = widget.NewSelect(nil, a.onSelectDiscoveredAgent)
	a.agentScanSelect.PlaceHolder = "Select detected ACP CLI on machine…"
	a.agentScanButton = widget.NewButtonWithIcon("Scan", theme.SearchIcon(), a.scanAndPopulateAgents)
	a.agentAddDiscoveredButton = widget.NewButton("Add", a.addSelectedDiscoveredAgent)

	scanControls := container.NewBorder(
		nil, nil,
		widget.NewLabelWithStyle("Discovered CLIs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(a.agentAddDiscoveredButton, a.agentScanButton),
		a.agentScanSelect,
	)

	a.agentSettingsPanel = container.NewVBox(
		widget.NewLabelWithStyle("ACP agents", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		a.agentSettingsSummary,
		widget.NewSeparator(),
		scanControls,
		widget.NewSeparator(),
		container.NewGridWithColumns(2, widget.NewLabel("ID"), a.agentIDEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Name"), a.agentNameEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Command"), a.agentCommandEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Arguments"), a.agentArgsEntry),
		container.NewHBox(save, restart, remove),
		widget.NewLabel("Save updates configuration. Click 'Restart agent' to apply changes immediately."),
	)
	a.agentSettingsPanel.Hide()
	a.renderACPAgentSettings()
}

func (a *application) toggleACPAgentPanel() {
	if a.window != nil {
		a.openACPAgentManagerDialog()
		return
	}
	if a.agentSettingsPanel.Visible() {
		a.agentSettingsPanel.Hide()
		return
	}
	a.runtimePanel.Hide()
	a.integrationPanel.Hide()
	a.populateACPAgentEditor(a.activeAgentID)
	a.agentSettingsPanel.Show()
	a.mu.Lock()
	needScan := len(a.discoveredAgents) == 0
	a.mu.Unlock()
	if needScan {
		go a.scanAndPopulateAgents()
	}
}

func (a *application) populateACPAgentEditor(agentID string) {
	a.mu.Lock()
	profile, ok := a.profiles[strings.TrimSpace(agentID)]
	a.mu.Unlock()
	if !ok {
		return
	}
	a.editingAgentID = profile.ID
	a.agentIDEntry.SetText(profile.ID)
	a.agentNameEntry.SetText(profile.DisplayName)
	a.agentCommandEntry.SetText(profile.Command.Path)
	args, _ := json.Marshal(profile.Command.Args)
	a.agentArgsEntry.SetText(string(args))
}

func (a *application) setDefaultAgent(id string) {
	id = strings.ToLower(strings.TrimSpace(id))
	a.mu.Lock()
	if _, ok := a.profiles[id]; !ok {
		a.mu.Unlock()
		return
	}
	a.activeAgentID = id
	if a.preferences != nil {
		a.preferences.SetString(activeAgentPreferencesKey, id)
	}
	a.mu.Unlock()
	a.refreshACPAgentSelector()
	a.renderACPAgentSettings()
	a.setStatus("Default agent for new sessions set to " + a.agentNameFor(id))
}

func (a *application) saveACPAgent() {
	id := strings.ToLower(strings.TrimSpace(a.agentIDEntry.Text))
	name := strings.TrimSpace(a.agentNameEntry.Text)
	command := strings.TrimSpace(a.agentCommandEntry.Text)
	if id == "" || name == "" || command == "" {
		a.setStatus("ACP agent requires ID, name, and command")
		return
	}
	args, err := parseStringList(a.agentArgsEntry.Text)
	if err != nil {
		a.setStatus("Invalid ACP agent arguments · " + err.Error())
		return
	}
	a.mu.Lock()
	profiles := make(map[string]agentProfile, len(a.profiles)+1)
	for key, profile := range a.profiles {
		profiles[key] = profile
	}
	if a.editingAgentID != "" && a.editingAgentID != id {
		delete(profiles, a.editingAgentID)
	}
	previous := profiles[id]
	profiles[id] = agentProfile{ID: id, DisplayName: name, Command: acpclient.CommandSpec{
		Path: command, Args: args, Env: previous.Command.Env,
	}}
	if err := persistACPAgentProfiles(a.preferences, profiles); err != nil {
		a.mu.Unlock()
		a.setStatus("Save ACP agent failed · " + err.Error())
		return
	}
	oldEditingID := a.editingAgentID
	a.profiles = profiles
	a.editingAgentID = id
	a.mu.Unlock()
	if oldEditingID != "" && oldEditingID != id {
		a.stopAgent(oldEditingID)
	}
	a.restartAgent(id)
	a.refreshACPAgentSelector()
	a.renderACPAgentSettings()
	a.setStatus("ACP agent saved and restarted")
}

func (a *application) openACPAgentManagerDialog(initialAgentID ...string) {
	if a.window == nil {
		return
	}

	selectedID := a.activeAgentID
	if len(initialAgentID) > 0 && initialAgentID[0] != "" {
		selectedID = initialAgentID[0]
	}

	statusBanner := widget.NewLabel("")
	statusBanner.Wrapping = fyne.TextWrapWord
	statusBanner.TextStyle = fyne.TextStyle{Bold: true}

	updateStatusBanner := func(id string) {
		a.mu.Lock()
		profile, hasProfile := a.profiles[id]
		health, hasHealth := a.state.AgentHealth[id]
		client := a.clients[id]
		isDefault := id == a.activeAgentID
		a.mu.Unlock()

		if !hasProfile {
			statusBanner.SetText("New Agent Profile (unsaved)")
			return
		}

		status := "connected"
		glyph := "●"
		if hasHealth {
			status = string(health.Status)
			if health.Status == desktopstate.AgentStatusConnecting {
				glyph = "○"
			} else if health.Status == desktopstate.AgentStatusDisconnected {
				glyph = "×"
			}
		} else if client == nil {
			status = "disconnected"
			glyph = "×"
		}

		capTag := "Standard ACP"
		if profile.ID == defaultAgentID {
			capTag = "Protonman Extensions (Goal, TODO, Memory, Runtime)"
		}

		defaultTag := ""
		if isDefault {
			defaultTag = " · Default for new sessions"
		}

		statusBanner.SetText(fmt.Sprintf("%s %s (%s) · %s%s", glyph, profile.DisplayName, status, capTag, defaultTag))
	}

	selectAgent := func(id string) {
		selectedID = id
		a.populateACPAgentEditor(id)
		updateStatusBanner(id)
	}

	listContainer := container.NewVBox()
	removeBtn := widget.NewButtonWithIcon("Remove", theme.DeleteIcon(), nil)
	setDefaultBtn := widget.NewButtonWithIcon("Set Default", theme.ConfirmIcon(), nil)

	var refreshList func()
	refreshList = func() {
		listContainer.Objects = nil

		a.mu.Lock()
		var sortedProfiles []agentProfile
		for _, p := range a.profiles {
			sortedProfiles = append(sortedProfiles, p)
		}
		sort.Slice(sortedProfiles, func(i, j int) bool {
			return sortedProfiles[i].DisplayName < sortedProfiles[j].DisplayName
		})
		activeID := a.activeAgentID
		agentHealth := a.state.AgentHealth
		a.mu.Unlock()

		listContainer.Add(widget.NewLabelWithStyle("Configured Profiles", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

		for _, p := range sortedProfiles {
			pID := p.ID
			pName := p.DisplayName
			glyph := "●"
			if h, ok := agentHealth[pID]; ok {
				if h.Status == desktopstate.AgentStatusConnecting {
					glyph = "○"
				} else if h.Status == desktopstate.AgentStatusDisconnected {
					glyph = "×"
				}
			}
			label := fmt.Sprintf("%s %s", glyph, pName)
			if pID == activeID {
				label += " (Default)"
			}
			targetID := pID
			btn := widget.NewButton(label, func() {
				selectAgent(targetID)
				refreshList()
			})
			if targetID == selectedID {
				btn.Importance = widget.HighImportance
			} else {
				btn.Importance = widget.LowImportance
			}
			listContainer.Add(btn)
		}

		addBtn := widget.NewButtonWithIcon("New Custom Agent", theme.ContentAddIcon(), func() {
			selectedID = ""
			a.editingAgentID = ""
			a.agentIDEntry.SetText("")
			a.agentNameEntry.SetText("")
			a.agentCommandEntry.SetText("")
			a.agentArgsEntry.SetText("[]")
			updateStatusBanner("")
			refreshList()
		})
		listContainer.Add(widget.NewSeparator())
		listContainer.Add(addBtn)
		listContainer.Refresh()

		a.mu.Lock()
		canRemove := len(a.profiles) > 1 && selectedID != ""
		a.mu.Unlock()
		if canRemove {
			removeBtn.Enable()
		} else {
			removeBtn.Disable()
		}
		if selectedID != "" && selectedID != activeID {
			setDefaultBtn.Enable()
		} else {
			setDefaultBtn.Disable()
		}
	}

	saveBtn := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		a.saveACPAgent()
		selectedID = strings.ToLower(strings.TrimSpace(a.agentIDEntry.Text))
		updateStatusBanner(selectedID)
		refreshList()
	})
	saveBtn.Importance = widget.HighImportance

	restartBtn := widget.NewButtonWithIcon("Restart", theme.ViewRefreshIcon(), func() {
		a.restartCurrentEditorAgent()
		updateStatusBanner(selectedID)
		refreshList()
	})

	setDefaultBtn.OnTapped = func() {
		if selectedID != "" {
			a.setDefaultAgent(selectedID)
			updateStatusBanner(selectedID)
			refreshList()
		}
	}

	removeBtn.OnTapped = func() {
		if selectedID != "" {
			a.removeACPAgent()
			a.mu.Lock()
			selectedID = a.activeAgentID
			a.mu.Unlock()
			selectAgent(selectedID)
			refreshList()
		}
	}

	browseBtn := widget.NewButtonWithIcon("Browse…", theme.FolderOpenIcon(), func() {
		if a.window == nil {
			return
		}
		fileOpen := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			_ = reader.Close()
			path := reader.URI().Path()
			if path != "" {
				a.agentCommandEntry.SetText(path)
			}
		}, a.window)
		fileOpen.Show()
	})

	commandField := container.NewBorder(nil, nil, nil, browseBtn, a.agentCommandEntry)

	formContainer := container.NewVBox(
		statusBanner,
		widget.NewSeparator(),
		container.NewGridWithColumns(2, widget.NewLabel("Agent ID"), a.agentIDEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Display Name"), a.agentNameEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Command / Executable"), commandField),
		container.NewGridWithColumns(2, widget.NewLabel("Arguments (JSON)"), a.agentArgsEntry),
		widget.NewSeparator(),
		container.NewHBox(saveBtn, restartBtn, setDefaultBtn, removeBtn),
		a.agentSettingsSummary,
	)

	discoveryBar := container.NewBorder(
		nil, nil,
		widget.NewLabelWithStyle("Discovered CLIs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(
			widget.NewButton("Add", func() {
				a.addSelectedDiscoveredAgent()
				selectedID = strings.ToLower(strings.TrimSpace(a.agentIDEntry.Text))
				updateStatusBanner(selectedID)
				refreshList()
			}),
			widget.NewButtonWithIcon("Scan", theme.SearchIcon(), func() {
				go func() {
					a.scanAndPopulateAgents()
					fyne.Do(func() {
						refreshList()
					})
				}()
			}),
		),
		a.agentScanSelect,
	)

	leftScroll := container.NewVScroll(listContainer)
	leftBox := container.New(fixedWidthLayout{width: 220}, leftScroll)
	detailSplit := container.NewBorder(nil, nil, leftBox, nil, container.NewPadded(formContainer))

	mainLayout := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("ACP Agent Manager", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			discoveryBar,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		detailSplit,
	)

	selectAgent(selectedID)
	refreshList()

	a.mu.Lock()
	needScan := len(a.discoveredAgents) == 0
	a.mu.Unlock()
	if needScan {
		go a.scanAndPopulateAgents()
	}

	d := dialog.NewCustom("Manage ACP Agents", "Close", container.New(
		minSizeLayout{width: 720, height: 440},
		mainLayout,
	), a.window)
	d.Resize(fyne.NewSize(740, 480))
	d.Show()
}


func (a *application) restartCurrentEditorAgent() {
	id := strings.ToLower(strings.TrimSpace(a.agentIDEntry.Text))
	if id == "" {
		return
	}
	a.setStatus("Restarting ACP agent · " + id)
	a.restartAgent(id)
}

func (a *application) removeACPAgent() {
	id := strings.ToLower(strings.TrimSpace(a.agentIDEntry.Text))
	a.mu.Lock()
	if len(a.profiles) <= 1 {
		a.mu.Unlock()
		a.setStatus("Keep at least one ACP agent configured")
		return
	}
	profiles := make(map[string]agentProfile, len(a.profiles)-1)
	for key, profile := range a.profiles {
		if key != id {
			profiles[key] = profile
		}
	}
	if len(profiles) == len(a.profiles) {
		a.mu.Unlock()
		a.setStatus("ACP agent not found · " + id)
		return
	}
	if err := persistACPAgentProfiles(a.preferences, profiles); err != nil {
		a.mu.Unlock()
		a.setStatus("Remove ACP agent failed · " + err.Error())
		return
	}
	a.profiles = profiles
	a.mu.Unlock()
	a.stopAgent(id)
	a.refreshACPAgentSelector()
	a.renderACPAgentSettings()
	a.setStatus("ACP agent removed · " + id)
}

func persistACPAgentProfiles(preferences preferenceWriter, profiles map[string]agentProfile) error {
	if preferences == nil {
		return fmt.Errorf("Desktop preferences are unavailable")
	}
	payload, err := json.Marshal(configuredAgentsFromProfiles(profiles))
	if err != nil {
		return err
	}
	preferences.SetString(agentProfilesPreferencesKey, string(payload))
	return nil
}

type preferenceWriter interface {
	preferenceReader
	SetString(string, string)
}

func (a *application) scanAndPopulateAgents() {
	if !a.isScanningAgents.CompareAndSwap(false, true) {
		return
	}
	defer a.isScanningAgents.Store(false)

	fyne.Do(func() {
		if a.agentScanButton != nil {
			a.agentScanButton.Disable()
		}
	})
	defer fyne.Do(func() {
		if a.agentScanButton != nil {
			a.agentScanButton.Enable()
		}
	})

	a.setStatus("Scanning machine for ACP-compatible CLIs…")
	discovered := ScanMachineACPAgents()

	a.mu.Lock()
	a.discoveredAgents = discovered
	options := make([]string, 0, len(discovered))
	for _, d := range discovered {
		options = append(options, d.OptionLabel())
	}
	a.mu.Unlock()

	fyne.Do(func() {
		if a.agentScanSelect != nil {
			a.agentScanSelect.Options = options
			if len(options) > 0 {
				a.mu.Lock()
				var pick string
				for _, d := range discovered {
					if _, ok := a.profiles[d.ID]; !ok {
						pick = d.OptionLabel()
						break
					}
				}
				if pick == "" && len(options) > 0 {
					pick = options[0]
				}
				a.mu.Unlock()
				a.agentScanSelect.SetSelected(pick)
			}
			a.agentScanSelect.Refresh()
		}
	})
	a.refreshACPAgentSelector()
	a.renderAgentStatus()
	a.setStatus(fmt.Sprintf("Scan complete · found %d ACP-compatible CLIs", len(discovered)))
}

func (a *application) onSelectDiscoveredAgent(label string) {
	a.mu.Lock()
	var selected *DiscoveredAgent
	for i := range a.discoveredAgents {
		if a.discoveredAgents[i].OptionLabel() == label {
			selected = &a.discoveredAgents[i]
			break
		}
	}
	a.mu.Unlock()

	if selected == nil {
		return
	}
	a.agentIDEntry.SetText(selected.ID)
	a.agentNameEntry.SetText(selected.DisplayName)
	a.agentCommandEntry.SetText(selected.Command)
	argsJSON, _ := json.Marshal(selected.Args)
	a.agentArgsEntry.SetText(string(argsJSON))
	a.setStatus("Loaded discovered CLI · " + selected.DisplayName + " (click 'Add' or 'Save' to configure)")
}

func (a *application) addSelectedDiscoveredAgent() {
	id := strings.ToLower(strings.TrimSpace(a.agentIDEntry.Text))
	name := strings.TrimSpace(a.agentNameEntry.Text)
	command := strings.TrimSpace(a.agentCommandEntry.Text)
	if id == "" || name == "" || command == "" {
		a.setStatus("Select a discovered agent or fill ID, Name, and Command")
		return
	}
	a.saveACPAgent()
}

func (a *application) agentOptionList() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	options := make([]string, 0, len(a.profiles)+len(a.discoveredAgents))
	for _, profile := range a.profiles {
		options = append(options, a.agentOptionLabel(profile))
	}
	sort.Strings(options)
	for _, d := range a.discoveredAgents {
		if _, ok := a.profiles[d.ID]; !ok {
			options = append(options, "+ Add "+d.DisplayName+" (detected)")
		}
	}
	options = append(options, "⚙ Manage ACP agents…")
	return options
}

func (a *application) agentOptionLabel(profile agentProfile) string {
	if profile.DisplayName != "" && profile.DisplayName != profile.ID {
		return profile.DisplayName
	}
	return profile.ID
}

func (a *application) agentOptionForID(id string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if profile, ok := a.profiles[id]; ok {
		return a.agentOptionLabel(profile)
	}
	return id
}

func (a *application) agentIDFromOption(option string) string {
	if strings.Contains(option, "Manage ACP agents") {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, profile := range a.profiles {
		if a.agentOptionLabel(profile) == option || id == option {
			return id
		}
	}
	for _, d := range a.discoveredAgents {
		if option == "+ Add "+d.DisplayName+" (detected)" || option == d.OptionLabel() {
			return d.ID
		}
	}
	return option
}

func (a *application) refreshACPAgentSelector() {
	options := a.agentOptionList()
	a.mu.Lock()
	if _, ok := a.profiles[a.activeAgentID]; !ok && len(a.profiles) > 0 {
		for id := range a.profiles {
			a.activeAgentID = id
			break
		}
	}
	active := a.activeAgentID
	a.mu.Unlock()

	if a.agentSelect != nil {
		a.agentSelect.Options = options
		a.agentSelect.SetSelected(a.agentOptionForID(active))
		a.agentSelect.Refresh()
	}
}

func (a *application) renderACPAgentSettings() {
	a.mu.Lock()
	count := len(a.profiles)
	active := a.activeAgentID
	profile, hasProfile := a.profiles[active]
	health, hasHealth := a.state.AgentHealth[active]
	client := a.clients[active]
	a.mu.Unlock()

	status := "connected"
	if hasHealth {
		status = string(health.Status)
	} else if client == nil {
		status = "disconnected"
	}

	name := active
	if hasProfile && profile.DisplayName != "" {
		name = profile.DisplayName
	}

	if a.agentSettingsSummary != nil {
		a.agentSettingsSummary.SetText(fmt.Sprintf("%d configured · new sessions use %s (%s)", count, name, status))
	}
}
