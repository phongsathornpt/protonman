//go:build desktop

package desktop

import (
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const (
	runtimeRefreshInterval = time.Second
	runtimeSummaryMaxRunes = 40
)

type sessionRuntimeResult struct {
	SessionID      string `json:"sessionId"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Reasoning      string `json:"reasoning"`
	LowConcurrency string `json:"lowConcurrency"`
}

type runtimeRefreshTracker struct {
	mu       sync.Mutex
	last     map[string]time.Time
	inFlight map[string]bool
}

var runtimeRefreshers sync.Map // map[*application]*runtimeRefreshTracker

func runtimeTrackerFor(a *application) *runtimeRefreshTracker {
	if existing, ok := runtimeRefreshers.Load(a); ok {
		return existing.(*runtimeRefreshTracker)
	}
	created := &runtimeRefreshTracker{last: make(map[string]time.Time), inFlight: make(map[string]bool)}
	actual, _ := runtimeRefreshers.LoadOrStore(a, created)
	return actual.(*runtimeRefreshTracker)
}

func (a *application) refreshSessionRuntime(sessionID string, force bool) {
	sessionID = strings.TrimSpace(sessionID)
	client := a.currentClient()
	if sessionID == "" || client == nil {
		return
	}
	tracker := runtimeTrackerFor(a)
	tracker.mu.Lock()
	if tracker.inFlight[sessionID] {
		tracker.mu.Unlock()
		return
	}
	if !force {
		if last := tracker.last[sessionID]; !last.IsZero() && time.Since(last) < runtimeRefreshInterval {
			tracker.mu.Unlock()
			return
		}
	}
	tracker.inFlight[sessionID] = true
	tracker.mu.Unlock()

	go func() {
		defer func() {
			tracker.mu.Lock()
			tracker.inFlight[sessionID] = false
			tracker.last[sessionID] = time.Now()
			tracker.mu.Unlock()
		}()
		var result sessionRuntimeResult
		if err := client.Call(a.ctx, "protonman/session/runtime", map[string]any{"sessionId": sessionID}, &result); err != nil {
			return
		}
		if !a.clientIsCurrent(client) || strings.TrimSpace(result.SessionID) != sessionID {
			return
		}
		a.applyRuntimeResult(result)
	}()
}

func (a *application) applyRuntimeResult(result sessionRuntimeResult) {
	a.mu.Lock()
	a.state = desktopstate.Reduce(a.state, desktopstate.Event{
		Kind:      desktopstate.EventSessionRuntimeUpdated,
		SessionID: result.SessionID,
		Runtime: desktopstate.RuntimeSettingsState{
			Provider:       strings.TrimSpace(result.Provider),
			Model:          strings.TrimSpace(result.Model),
			Reasoning:      strings.TrimSpace(result.Reasoning),
			LowConcurrency: strings.TrimSpace(result.LowConcurrency),
		},
	})
	active := a.state.ActiveSessionID == result.SessionID
	a.mu.Unlock()
	if active {
		a.renderRuntimeControls()
	}
}

func (a *application) setRuntimeModel() {
	a.mu.Lock()
	sessionID := a.state.ActiveSessionID
	busy := a.sessionBusyLocked(sessionID)
	a.mu.Unlock()
	client := a.currentClient()
	provider := strings.TrimSpace(a.modelProvider.Text)
	modelID := strings.TrimSpace(a.modelID.Text)
	if client == nil || sessionID == "" || busy || provider == "" || modelID == "" {
		return
	}
	go func() {
		var result sessionRuntimeResult
		if err := client.Call(a.ctx, "protonman/session/set_model", map[string]any{"sessionId": sessionID, "provider": provider, "model": modelID}, &result); err != nil {
			a.setStatus("Model update failed · " + err.Error())
			return
		}
		a.applyRuntimeResult(result)
	}()
}

func (a *application) setRuntimeReasoning(value string) {
	a.setRuntimeChoice("protonman/session/set_reasoning", "reasoning", value)
}

func (a *application) setRuntimeLowConcurrency(value string) {
	a.setRuntimeChoice("protonman/session/set_low_concurrency", "lowConcurrency", value)
}

func (a *application) setRuntimeChoice(method, field, value string) {
	a.mu.Lock()
	sessionID := a.state.ActiveSessionID
	busy := a.sessionBusyLocked(sessionID)
	a.mu.Unlock()
	client := a.currentClient()
	value = strings.TrimSpace(value)
	if client == nil || sessionID == "" || busy || value == "" {
		return
	}
	go func() {
		params := map[string]any{"sessionId": sessionID, field: value}
		var result sessionRuntimeResult
		if err := client.Call(a.ctx, method, params, &result); err != nil {
			a.setStatus("Runtime update failed · " + err.Error())
			a.refreshSessionRuntime(sessionID, true)
			return
		}
		a.applyRuntimeResult(result)
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
			a.modelProvider.Disable()
			a.modelID.Disable()
			a.applyModel.Disable()
			a.reasoningSelect.Disable()
			a.lowSelect.Disable()
		} else {
			a.modelProvider.Enable()
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
		base = strings.TrimSpace(runtime.Provider)
	}
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
