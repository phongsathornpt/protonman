package runtime

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func (m *bubbleModel) handleLowConcurrencyCommand(argument string) tea.Cmd {
	if m.busy {
		m.appendError("cannot change low concurrency mode while a turn is running")
		return nil
	}

	raw := strings.TrimSpace(argument)
	var next model.LowConcurrencySetting
	var err error
	if raw == "" {
		if m.lowConcurrencyMode == model.LowConcurrencyOn {
			next = model.LowConcurrencyOff
		} else {
			next = model.LowConcurrencyOn
		}
	} else {
		next, err = model.ParseLowConcurrencySetting(raw)
		if err != nil {
			m.appendError(err.Error())
			return nil
		}
	}

	m.lowConcurrencyMode = next
	m.reconfigureRunner()
	m.appendLine(mutedStyle.Render(fmt.Sprintf("  low concurrency · %s", next)))
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) lowConcurrencyFooterLabel() string {
	providerName := strings.TrimSpace(m.activeProvider)
	if providerName == "" {
		providerName = model.DefaultOpenCodeName
	}
	if strings.EqualFold(providerName, model.DefaultOpenCodeName) {
		return "low:" + m.lowConcurrencyMode.String()
	}
	provider, ok := m.providers[strings.ToLower(providerName)]
	if !ok || !model.IsProvider(model.DefaultOpenCodeName, providerName, provider.BaseURL) {
		return ""
	}
	return "low:" + m.lowConcurrencyMode.String()
}
