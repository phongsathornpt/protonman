package runtime

import (
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/app"
)

func (v *providerSelectPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
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
		return true, cmd
	}
	if v.deleteConfirm {
		switch message.String() {
		case "enter":
			item, ok := v.selectedItem()
			v.deleteConfirm = false
			if !ok {
				return true, nil
			}
			m.bottom.remove(providerSelectViewID)
			return true, m.beginProviderDelete(item.name)
		case "esc":
			v.deleteConfirm = false
			return true, nil
		case "ctrl+c":
			v.deleteConfirm = false
			m.bottom.remove(providerSelectViewID)
			return true, nil
		default:
			return !m.matchesGlobalShortcut(message), nil
		}
	}
	switch message.String() {
	case "esc", "q":
		m.bottom.remove(providerSelectViewID)
		return true, nil
	case "a", "c":
		m.bottom.remove(providerSelectViewID)
		if !m.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneView())
		}
		return true, nil
	case "m":
		if item, ok := v.selectedItem(); ok {
			if item.isConfigured {
				m.bottom.remove(providerSelectViewID)
				if !m.bottom.has(modelSelectViewID) {
					mv := newModelSelectPaneView(m)
					for i, name := range mv.providerNames {
						if strings.EqualFold(name, item.name) {
							mv.providerIndex = i
							break
						}
					}
					m.bottom.push(mv)
					if _, ok := m.providers[strings.ToLower(item.name)]; ok {
						return true, mv.loadProvider(m, false)
					}
				}
				return true, nil
			}
		}
		return true, nil
	case "e":
		if item, ok := v.selectedItem(); ok {
			m.bottom.remove(providerSelectViewID)
			if !m.bottom.has(providerViewID) {
				if item.isConfigured {
					if cfg, ok := m.providers[strings.ToLower(item.name)]; ok {
						pv := newProviderPaneViewWithConfig(cfg)
						pv.activateOnSave = item.isActive
						m.pushProviderPane(pv)
					} else {
						pv := newProviderPaneViewWithPreset(item.name)
						pv.activateOnSave = item.isActive
						m.pushProviderPane(pv)
					}
				} else if item.kind == providerItemPreset {
					m.pushProviderPane(newProviderPaneViewWithPreset(item.presetID))
				} else {
					m.pushProviderPane(newProviderPaneView())
				}
			}
			return true, nil
		}
		return true, nil
	case "d":
		if item, ok := v.selectedItem(); ok {
			if item.isConfigured {
				v.deleteConfirm = true
			}
		}
		return true, nil
	case "/":
		v.picker.SetFilterState(list.Filtering)
		return true, nil
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "g", "end", "G":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return true, cmd
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		return true, nil
	case "enter":
		if item, ok := v.selectedItem(); ok {
			m.bottom.remove(providerSelectViewID)
			if item.isConfigured {
				return true, m.beginProviderSelect(item.name)
			}
			if item.kind == providerItemPreset {
				if !m.bottom.has(providerViewID) {
					m.pushProviderPane(newProviderPaneViewWithPreset(item.presetID))
				}
				return true, nil
			}
			if !m.bottom.has(providerViewID) {
				m.pushProviderPane(newProviderPaneView())
			}
			return true, nil
		}
		return true, nil
	default:
		return false, nil
	}
}

func saveActiveProviderCmd(operationID asyncOperationID, providerName string) tea.Cmd {
	return func() tea.Msg {
		err := (app.Providers{}).Select(providerName)
		return providerActiveSelectedMsg{operationID: operationID, providerName: providerName, err: err}
	}
}

func deleteProviderCmd(operationID asyncOperationID, providerName string) tea.Cmd {
	return func() tea.Msg {
		err := (app.Providers{}).Delete(providerName)
		return providerDeletedMsg{operationID: operationID, providerName: providerName, err: err}
	}
}
