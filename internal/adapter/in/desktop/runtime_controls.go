//go:build desktop

package desktop

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const runtimeSummaryMaxRunes = 40

// refreshSessionRuntime is intentionally a no-op. Runtime configuration is now
// delivered by ACP session/new and session/resume results, then refreshed by
// session/set_config_option responses. Keeping this hook temporarily avoids
// coupling unrelated conversation refresh code to the migration.
func (a *application) refreshSessionRuntime(_ string, _ bool) {}

func cloneSessionConfigOptions(options []acpclient.SessionConfigOption) []acpclient.SessionConfigOption {
	out := append([]acpclient.SessionConfigOption(nil), options...)
	for i := range out {
		out[i].Options = append([]acpclient.SessionConfigSelectOption(nil), out[i].Options...)
	}
	return out
}

func (a *application) applySessionConfigOptions(sessionID string, options []acpclient.SessionConfigOption) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	a.mu.Lock()
	if a.sessionConfigOptions == nil {
		a.sessionConfigOptions = make(map[string][]acpclient.SessionConfigOption)
	}
	a.sessionConfigOptions[sessionID] = cloneSessionConfigOptions(options)
	var runtime desktopstate.RuntimeSettingsState
	for _, session := range a.state.Sessions {
		if session.ID == sessionID {
			runtime = session.Runtime
			break
		}
	}
	if len(options) > 0 {
		runtime.Provider = "ACP"
	}
	for _, option := range options {
		switch strings.TrimSpace(option.ID) {
		case "model":
			runtime.Model = strings.TrimSpace(option.CurrentValue)
		case "reasoning":
			runtime.Reasoning = strings.TrimSpace(option.CurrentValue)
		case "lowConcurrency":
			runtime.LowConcurrency = strings.TrimSpace(option.CurrentValue)
		}
	}
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{
		Kind:      desktopstate.EventSessionRuntimeUpdated,
		SessionID: sessionID,
		Runtime:   runtime,
	})
	active := a.state.ActiveSessionID == sessionID
	a.mu.Unlock()

	if active {
		a.renderRuntimeControls()
	}
}

func (a *application) setRuntimeModel(value string) {
	a.setSessionConfigOption("model", value)
}

func (a *application) setRuntimeReasoning(value string) {
	a.setSessionConfigOption("reasoning", value)
}

func (a *application) setRuntimeLowConcurrency(value string) {
	a.setSessionConfigOption("lowConcurrency", value)
}

func (a *application) setSessionConfigOption(configID, value string) {
	a.mu.Lock()
	sessionID := a.state.ActiveSessionID
	busy := a.sessionBusyLocked(sessionID)
	a.mu.Unlock()

	client := a.currentClient()
	configID = strings.TrimSpace(configID)
	value = strings.TrimSpace(value)
	if client == nil || sessionID == "" || busy || configID == "" || value == "" {
		return
	}

	go func() {
		var result acpclient.SessionSetConfigOptionResult
		if err := client.Call(a.ctx, "session/set_config_option", map[string]any{
			"sessionId": sessionID,
			"configId":  configID,
			"type":      "select",
			"value":     value,
		}, &result); err != nil {
			if a.clientIsCurrent(client) {
				a.setStatus("Runtime update failed · " + err.Error())
			}
			return
		}
		if !a.clientIsCurrent(client) {
			return
		}
		a.applySessionConfigOptions(sessionID, result.ConfigOptions)
	}()
}

func sessionConfigValues(options []acpclient.SessionConfigOption, configID, current string) []string {
	var values []string
	for _, option := range options {
		if strings.TrimSpace(option.ID) != configID {
			continue
		}
		values = make([]string, 0, len(option.Options)+1)
		for _, candidate := range option.Options {
			value := strings.TrimSpace(candidate.Value)
			if value != "" {
				values = append(values, value)
			}
		}
		break
	}
	current = strings.TrimSpace(current)
	if current != "" {
		found := false
		for _, value := range values {
			if value == current {
				found = true
				break
			}
		}
		if !found {
			values = append([]string{current}, values...)
		}
	}
	return values
}

func (a *application) renderRuntimeControls() {
	a.mu.Lock()
	activeID := a.state.ActiveSessionID
	busy := a.sessionBusyLocked(activeID)
	var runtime desktopstate.RuntimeSettingsState
	for _, session := range a.state.Sessions {
		if session.ID == activeID {
			runtime = session.Runtime
			break
		}
	}
	options := cloneSessionConfigOptions(a.sessionConfigOptions[activeID])
	a.mu.Unlock()

	modelValues := sessionConfigValues(options, "model", runtime.Model)
	reasoningValues := sessionConfigValues(options, "reasoning", runtime.Reasoning)
	lowValues := sessionConfigValues(options, "lowConcurrency", runtime.LowConcurrency)
	summary := runtimeSummaryText(runtime)

	fyne.Do(func() {
		a.runtimeSync = true
		a.modelProvider.SetText("ACP")
		a.modelProvider.Disable()

		a.modelSelect.Options = modelValues
		a.modelSelect.Refresh()
		if runtime.Model != "" {
			a.modelSelect.SetSelected(runtime.Model)
		} else {
			a.modelSelect.ClearSelected()
		}

		a.reasoningSelect.Options = reasoningValues
		a.reasoningSelect.Refresh()
		if runtime.Reasoning != "" {
			a.reasoningSelect.SetSelected(runtime.Reasoning)
		} else {
			a.reasoningSelect.ClearSelected()
		}

		a.lowSelect.Options = lowValues
		a.lowSelect.Refresh()
		if runtime.LowConcurrency != "" {
			a.lowSelect.SetSelected(runtime.LowConcurrency)
		} else {
			a.lowSelect.ClearSelected()
		}
		a.runtimeSummary.SetText(summary)
		a.runtimeSync = false

		setSelectEnabled(a.modelSelect, activeID != "" && !busy && len(modelValues) > 0)
		setSelectEnabled(a.reasoningSelect, activeID != "" && !busy && len(reasoningValues) > 0)
		setSelectEnabled(a.lowSelect, activeID != "" && !busy && len(lowValues) > 0)
	})
}

func setSelectEnabled(selectWidget *widget.Select, enabled bool) {
	if enabled {
		selectWidget.Enable()
		return
	}
	selectWidget.Disable()
}

func runtimeSummaryText(runtime desktopstate.RuntimeSettingsState) string {
	base := strings.TrimSpace(runtime.Model)
	if base == "" {
		base = "Model"
	}

	suffix := ""
	if reasoning := strings.TrimSpace(runtime.Reasoning); reasoning != "" && reasoning != "auto" {
		suffix += " · " + reasoning
	}
	if low := strings.TrimSpace(runtime.LowConcurrency); low == "on" {
		suffix += " · low"
	}

	baseLimit := runtimeSummaryMaxRunes - len([]rune(suffix))
	if baseLimit < 8 {
		baseLimit = 8
	}
	return compactText(base, baseLimit) + suffix
}
