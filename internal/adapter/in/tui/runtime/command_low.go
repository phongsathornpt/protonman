package runtime

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func (m *bubbleModel) handleLowConcurrencyCommand(argument string) tea.Cmd {
	raw := strings.TrimSpace(argument)
	if raw == "" {
		m.openLowConcurrencyPane()
		return nil
	}
	if m.busy {
		m.appendError("cannot change low concurrency mode while a turn is running")
		return nil
	}

	next, err := model.ParseLowConcurrencySetting(raw)
	if err != nil {
		m.appendError(err.Error())
		return nil
	}
	return m.applyLowConcurrencySetting(next)
}

func (m *bubbleModel) applyLowConcurrencySetting(next model.LowConcurrencySetting) tea.Cmd {
	if m.busy {
		m.appendError("cannot change low concurrency mode while a turn is running")
		return nil
	}
	m.lowConcurrencyMode = next
	m.reconfigureRunner()
	effective := "off"
	if m.lowConcurrencyEffective() {
		effective = "on"
	}
	m.refreshViewport()
	return m.showTransientNotice(fmt.Sprintf("low concurrency · %s · effective %s", next, effective))
}

func (m *bubbleModel) lowConcurrencyEffective() bool {
	if strings.TrimSpace(m.activeModel) == "" {
		return false
	}
	if m.lowConcurrencyMode == model.LowConcurrencyOn {
		return true
	}
	if m.lowConcurrencyMode == model.LowConcurrencyOff {
		return false
	}
	providerName := strings.TrimSpace(m.activeProvider)
	if providerName == "" {
		providerName = model.DefaultOpenCodeName
	}
	provider, ok := m.providers[strings.ToLower(providerName)]
	baseURL := ""
	if ok {
		baseURL = provider.BaseURL
	}
	return model.IsProvider(model.DefaultOpenCodeName, providerName, baseURL) && model.IsFreeModel(m.activeModel)
}

func (m *bubbleModel) lowConcurrencyFooterLabel() string {
	if m.lowConcurrencyEffective() {
		return "LOW"
	}
	return ""
}
