package runtime

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/app"
)

func (v *providerSelectPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	v.initPicker()
	if v.deleteConfirm {
		item, ok := v.selectedItem()
		if !ok || !item.isConfigured {
			v.deleteConfirm = false
		}
	}
	if v.picker.SettingFilter() {
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	}
	if v.deleteConfirm {
		switch message.String() {
		case "enter":
			item, ok := v.selectedItem()
			v.deleteConfirm = false
			if !ok {
				return paneKeyResult{handled: true}
			}
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderDelete, providerItem: item}}
		case "esc":
			v.deleteConfirm = false
			return paneKeyResult{handled: true}
		case "ctrl+c":
			v.deleteConfirm = false
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: providerSelectViewID}}
		default:
			return paneKeyResult{handled: true}
		}
	}
	switch message.String() {
	case "esc", "q":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: providerSelectViewID}}
	case "a", "c":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionOpenProviderEditor}}
	case "m":
		if item, ok := v.selectedItem(); ok && item.isConfigured {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderModels, providerItem: item}}
		}
		return paneKeyResult{handled: true}
	case "e":
		if item, ok := v.selectedItem(); ok {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderEdit, providerItem: item}}
		}
		return paneKeyResult{handled: true}
	case "d":
		if item, ok := v.selectedItem(); ok && item.isConfigured {
			v.deleteConfirm = true
		}
		return paneKeyResult{handled: true}
	case "/":
		v.picker.SetFilterState(list.Filtering)
		return paneKeyResult{handled: true}
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return paneKeyResult{handled: true}
	case "enter":
		item, ok := v.selectedItem()
		if !ok {
			return paneKeyResult{handled: true}
		}
		if item.isConfigured {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderActivate, providerItem: item}}
		}
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderEdit, providerItem: item}}
	default:
		return paneKeyResult{}
	}
}

func saveActiveProviderCmd(operationID asyncOperationID, gate *asyncOperationGate, providerName, reconciledModel string) tea.Cmd {
	return func() tea.Msg {
		if !gate.current(operationID) {
			return providerActiveSelectedMsg{operationID: operationID, providerName: providerName, reconciledModel: reconciledModel, err: errStaleConfigMutation}
		}
		err := (app.Providers{}).Activate(providerName, reconciledModel)
		return providerActiveSelectedMsg{operationID: operationID, providerName: providerName, reconciledModel: reconciledModel, err: err}
	}
}

func deleteProviderCmd(operationID asyncOperationID, gate *asyncOperationGate, providerName string) tea.Cmd {
	return func() tea.Msg {
		if !gate.current(operationID) {
			return providerDeletedMsg{operationID: operationID, providerName: providerName, err: errStaleConfigMutation}
		}
		err := (app.Providers{}).Delete(providerName)
		return providerDeletedMsg{operationID: operationID, providerName: providerName, err: err}
	}
}
