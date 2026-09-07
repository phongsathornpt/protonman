package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/model"
)

func (m *bubbleModel) selectModelDirect(modelID string) tea.Cmd {
	prov := m.activeProvider
	if prov == "" {
		if len(m.providers) > 0 {
			for name := range m.providers {
				prov = name
				break
			}
		} else {
			prov = model.DefaultProtonmanName
		}
	}
	return saveModelSelectionCmd(prov, modelID, !m.modelIDKnown(prov, modelID))
}
