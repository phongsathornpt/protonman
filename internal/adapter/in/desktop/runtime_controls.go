//go:build desktop

package desktop

import (
	"strings"

	"fyne.io/fyne/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const runtimeSummaryMaxRunes = 40

// refreshSessionRuntime is intentionally a no-op. Runtime configuration is now
// delivered by ACP session/new and session/resume results, then refreshed by
// session/set_config_option responses. Keeping this hook temporarily avoids
// coupling unrelated conversation refresh code to the migration.
func (a *application) refreshSessionRuntime(_ string, _ bool) {}

func (a *application) applySessionConfigOptions(sessionID string, options []acpclient.SessionConfigOption) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	a.mu.Lock()
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

func (a *application) setRuntimeModel() {
	value := strings.TrimSpace(a.modelID.Text)
	if value == "" {
		return
	}
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
	a.mu.Unlock()
	summary := runtimeSummaryText(runtime)

	fyne.Do(func() {
		a.runtimeSync = true
		a.modelProvider.SetText(runtime.Provider)
		a.modelProvider.Disable()
		a.modelID.SetText(runtime.Model)
		if runtime.Reasoning != "" {
			a.reasoningSelect.SetSelected(runtime.Reasoning)
		}
		if runtime.LowConcurrency != "" {
			a.lowSelect.SetSelected(runtime.LowConcurrency)
		}
		a.runtimeSummary.SetText(summary)
		a.runtimeSync = false
		if activeID == "" || busy {
			a.modelID.Disable()
			a.applyModel.Disable()
			a.reasoningSelect.Disable()
			a.lowSelect.Disable()
		} else {
			a.modelID.Enable()
			a.applyModel.Enable()
			a.reasoningSelect.Enable()
			a.lowSelect.Enable()
		}
	})
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
