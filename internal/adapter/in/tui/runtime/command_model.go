package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func (m *bubbleModel) selectModelDirect(modelID string) tea.Cmd {
	providerName := strings.TrimSpace(m.activeProvider)
	if providerName == "" {
		providerName = model.DefaultOpenCodeName
	}
	return m.beginModelSetup(providerName, modelID, !m.modelIDKnown(providerName, modelID))
}

func (m *bubbleModel) executeModelCommand(argument string) tea.Cmd {
	switch arg := strings.TrimSpace(argument); arg {
	case "add":
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneView())
			m.requestRelayout()
		}
		return nil
	case "free":
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneViewWithPreset(model.DefaultOpenCodeName))
			m.requestRelayout()
		}
		return nil
	case "", "select":
		return m.openModelSetupPane()
	default:
		return m.selectModelDirect(arg)
	}
}
