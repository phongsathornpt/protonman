//go:build desktop

package desktop

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
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
	remove := widget.NewButton("Remove agent", a.removeACPAgent)
	a.agentSettingsPanel = container.NewVBox(
		widget.NewLabelWithStyle("ACP agents", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		a.agentSettingsSummary,
		container.NewGridWithColumns(2, widget.NewLabel("ID"), a.agentIDEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Name"), a.agentNameEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Command"), a.agentCommandEntry),
		container.NewGridWithColumns(2, widget.NewLabel("Arguments"), a.agentArgsEntry),
		container.NewHBox(save, remove),
		widget.NewLabel("Changes apply after restarting Desktop. Environment values are not stored."),
	)
	a.agentSettingsPanel.Hide()
	a.renderACPAgentSettings()
}

func (a *application) toggleACPAgentPanel() {
	if a.agentSettingsPanel.Visible() {
		a.agentSettingsPanel.Hide()
		return
	}
	a.runtimePanel.Hide()
	a.integrationPanel.Hide()
	a.populateACPAgentEditor(a.activeAgentID)
	a.agentSettingsPanel.Show()
}

func (a *application) populateACPAgentEditor(agentID string) {
	a.mu.Lock()
	profile, ok := a.profiles[strings.TrimSpace(agentID)]
	a.mu.Unlock()
	if !ok {
		return
	}
	a.agentIDEntry.SetText(profile.ID)
	a.agentNameEntry.SetText(profile.DisplayName)
	a.agentCommandEntry.SetText(profile.Command.Path)
	args, _ := json.Marshal(profile.Command.Args)
	a.agentArgsEntry.SetText(string(args))
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
	oldID := strings.TrimSpace(a.agentIDEntry.Text)
	if oldID != id {
		delete(profiles, oldID)
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
	a.profiles = profiles
	a.mu.Unlock()
	a.refreshACPAgentSelector()
	a.renderACPAgentSettings()
	a.setStatus("ACP agent saved · restart Desktop to connect changes")
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
	a.refreshACPAgentSelector()
	a.renderACPAgentSettings()
	a.setStatus("ACP agent removed · restart Desktop to apply")
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

func (a *application) refreshACPAgentSelector() {
	a.mu.Lock()
	ids := make([]string, 0, len(a.profiles))
	for id := range a.profiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if _, ok := a.profiles[a.activeAgentID]; !ok && len(ids) > 0 {
		a.activeAgentID = ids[0]
	}
	active := a.activeAgentID
	a.mu.Unlock()
	a.agentSelect.Options = ids
	a.agentSelect.SetSelected(active)
	a.agentSelect.Refresh()
}

func (a *application) renderACPAgentSettings() {
	a.mu.Lock()
	count := len(a.profiles)
	active := a.activeAgentID
	a.mu.Unlock()
	if a.agentSettingsSummary != nil {
		a.agentSettingsSummary.SetText(fmt.Sprintf("%d configured · new sessions use %s", count, active))
	}
}
