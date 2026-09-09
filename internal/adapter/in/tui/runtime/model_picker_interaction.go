package runtime

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/providerio"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"strings"
)

func (v *modelSelectPaneView) Render(m *bubbleModel) string {
	v.initPicker()
	if m == nil {
		return ""
	}
	providerName := v.activeProviderName()
	v.picker.Title = "Select Model · " + providerName
	if len(v.providerNames) > 1 {
		v.picker.Title += " · tab provider"
	}
	v.picker.SetSize(maxInt(12, m.width-8), maxInt(5, min(16, m.height-4)))
	mode := layoutModeForHeight(m.height)
	v.picker.SetShowStatusBar(mode == layoutNormal)
	v.picker.SetShowPagination(mode != layoutTiny)
	v.picker.SetShowHelp(mode != layoutTiny)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.picker.SetDelegate(delegate)
	if v.loading {
		rows := []string{brandStyle.Render("Select Model · " + providerName), "", mutedStyle.Render("Loading models..."), "", mutedStyle.Render("esc close")}
		return renderModalRows(m, accentAssistant, rows)
	}
	if v.err != nil {
		rows := []string{brandStyle.Render("Select Model · " + providerName), "", errorStyle.Render("Failed to load models"), mutedStyle.Render(truncateWithEllipsis(v.err.Error(), maxInt(8, m.width-8))), "", mutedStyle.Render("r retry · p providers · esc close")}
		return renderModalRows(m, accentAssistant, rows)
	}
	if len(v.picker.Items()) == 0 && !v.picker.SettingFilter() && !v.picker.IsFiltered() {
		rows := []string{brandStyle.Render("Select Model · " + providerName), "", mutedStyle.Render("No models available for the selected provider."), "", mutedStyle.Render("a add provider · r retry · esc close")}
		return renderModalRows(m, accentAssistant, rows)
	}
	if len(v.picker.VisibleItems()) == 0 && strings.TrimSpace(v.picker.FilterValue()) != "" {
		rows := []string{brandStyle.Render("Select Model · " + providerName), mutedStyle.Render("Search: " + v.picker.FilterValue()), "", mutedStyle.Render("No models match the current search."), "", mutedStyle.Render("esc clear filter")}
		return renderModalRows(m, accentAssistant, rows)
	}
	return renderModalRows(m, accentAssistant, strings.Split(v.picker.View(), "\n"))
}

func (v *modelSelectPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	if key.Matches(message, m.keys.ToggleModel) {
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil
	}
	v.initPicker()
	if v.picker.SettingFilter() {
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		return true, cmd
	}
	switch message.String() {
	case "/":
		v.picker.SetFilterState(list.Filtering)
		v.syncPickerProjection()
		return true, nil
	case "ctrl+u":
		v.picker.ResetFilter()
		v.syncPickerProjection()
		return true, nil
	case "esc":
		if v.picker.IsFiltered() {
			v.picker.ResetFilter()
			v.syncPickerProjection()
			return true, nil
		}
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil
	case "q":
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		return true, nil
	case "p":
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		if !m.bottom.has(providerSelectViewID) {
			m.bottom.push(newProviderSelectPaneView(m))
		}
		return true, nil
	case "a":
		v.cancelFetch()
		m.bottom.remove(modelSelectViewID)
		if !m.bottom.has(providerViewID) {
			m.bottom.push(newProviderPaneView())
		}
		return true, nil
	case "r":
		return true, v.loadProvider(m, true)
	case "tab":
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex + 1) % len(v.providerNames)
			return true, v.loadProvider(m, false)
		}
		return true, nil
	case "shift+tab":
		if len(v.providerNames) > 1 {
			v.providerIndex = (v.providerIndex - 1 + len(v.providerNames)) % len(v.providerNames)
			return true, v.loadProvider(m, false)
		}
		return true, nil
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		v.syncPickerProjection()
		return true, cmd
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		pageOffset := v.picker.Paginator.Page * v.picker.Paginator.PerPage
		targetIdx := int(message.String()[0]-'1') + pageOffset
		if targetIdx >= 0 && targetIdx < len(v.models) {
			selected := v.models[targetIdx]
			provName := model.DefaultProtonmanName
			if v.providerIndex >= 0 && v.providerIndex < len(v.providerNames) {
				provName = v.providerNames[v.providerIndex]
			}
			cmd := saveDefaultModelCmd(provName, selected.ID)
			m.bottom.remove(modelSelectViewID)
			return true, cmd
		}
		return true, nil
	case "enter":
		item, ok := v.picker.SelectedItem().(modelListItem)
		if !ok {
			return true, nil
		}
		provName := model.DefaultProtonmanName
		if v.providerIndex >= 0 && v.providerIndex < len(v.providerNames) {
			provName = v.providerNames[v.providerIndex]
		}
		cmd := saveDefaultModelCmd(provName, item.model.ID)
		m.bottom.remove(modelSelectViewID)
		return true, cmd
	default:
		return false, nil
	}
}

func saveDefaultModelCmd(providerName, modelID string) tea.Cmd {
	return saveModelSelectionCmd(providerName, modelID, false)
}

func saveModelSelectionCmd(providerName, modelID string, unverified bool) tea.Cmd {
	return func() tea.Msg {
		err := providerio.SelectModel(providerName, modelID)
		return modelSelectedMsg{providerName: providerName, modelID: modelID, unverified: unverified, err: err}
	}
}
