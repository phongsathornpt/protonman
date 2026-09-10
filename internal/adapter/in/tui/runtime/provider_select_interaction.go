package runtime

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/app"
)

var providerSelectKeys = struct {
	Add, Models, Edit, Delete, Filter key.Binding
}{
	Add:    key.NewBinding(key.WithKeys("a", "c")),
	Models: key.NewBinding(key.WithKeys("m")),
	Edit:   key.NewBinding(key.WithKeys("e")),
	Delete: key.NewBinding(key.WithKeys("d")),
	Filter: key.NewBinding(key.WithKeys("/")),
}

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
		switch {
		case key.Matches(message, paneKeys.Confirm):
			item, ok := v.selectedItem()
			v.deleteConfirm = false
			if !ok {
				return paneKeyResult{handled: true}
			}
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderDelete, providerItem: item}}
		case key.Matches(message, paneKeys.Escape):
			v.deleteConfirm = false
			return paneKeyResult{handled: true}
		default:
			return paneKeyResult{handled: true}
		}
	}
	switch {
	case key.Matches(message, paneKeys.Close):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: providerSelectViewID}}
	case key.Matches(message, providerSelectKeys.Add):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionOpenProviderEditor}}
	case key.Matches(message, providerSelectKeys.Models):
		if item, ok := v.selectedItem(); ok && item.isConfigured {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderModels, providerItem: item}}
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, providerSelectKeys.Edit):
		if item, ok := v.selectedItem(); ok {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionProviderEdit, providerItem: item}}
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, providerSelectKeys.Delete):
		if item, ok := v.selectedItem(); ok && item.isConfigured {
			v.deleteConfirm = true
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, providerSelectKeys.Filter):
		v.picker.SetFilterState(list.Filtering)
		return paneKeyResult{handled: true}
	case key.Matches(message, paneKeys.Nav):
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	case message.Text >= "1" && message.Text <= "9":
		return paneKeyResult{handled: true}
	case key.Matches(message, paneKeys.Confirm):
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
