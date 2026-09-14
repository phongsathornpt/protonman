//go:build desktop

package desktop

import (
	"strings"
	"sync"
)

type sessionHistoryState uint8

const (
	sessionHistoryUnloaded sessionHistoryState = iota
	sessionHistoryLoading
	sessionHistoryLoaded
)

type sessionHistoryTracker struct {
	mu     sync.Mutex
	states map[string]sessionHistoryState
}

var sessionHistoryTrackers sync.Map // map[*application]*sessionHistoryTracker

func sessionHistoryTrackerFor(a *application) *sessionHistoryTracker {
	if existing, ok := sessionHistoryTrackers.Load(a); ok {
		return existing.(*sessionHistoryTracker)
	}
	created := &sessionHistoryTracker{states: make(map[string]sessionHistoryState)}
	actual, _ := sessionHistoryTrackers.LoadOrStore(a, created)
	return actual.(*sessionHistoryTracker)
}

func sessionHistoryIsLoading(a *application, sessionID string) bool {
	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.states[sessionID] == sessionHistoryLoading
}

func (a *application) submitPrompt() {
	a.mu.Lock()
	sessionID := a.state.ActiveSessionID
	a.mu.Unlock()
	if sessionID == "" || sessionHistoryIsLoading(a, sessionID) {
		return
	}
	a.sendPrompt()
}

func (a *application) loadSessionHistory(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	client := a.currentClient()
	if sessionID == "" || client == nil {
		return
	}

	a.mu.Lock()
	workspace := ""
	transcriptPresent := false
	for _, session := range a.state.Sessions {
		if session.ID == sessionID {
			workspace = validWorkspacePath(session.Workspace)
			break
		}
	}
	if transcript := a.transcripts[sessionID]; transcript != nil && transcript.Len() > 0 {
		transcriptPresent = true
	}
	a.mu.Unlock()

	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	if transcriptPresent {
		tracker.states[sessionID] = sessionHistoryLoaded
		tracker.mu.Unlock()
		return
	}
	if state := tracker.states[sessionID]; state == sessionHistoryLoading || state == sessionHistoryLoaded {
		tracker.mu.Unlock()
		return
	}
	tracker.states[sessionID] = sessionHistoryLoading
	tracker.mu.Unlock()

	if workspace == "" {
		tracker.mu.Lock()
		tracker.states[sessionID] = sessionHistoryUnloaded
		tracker.mu.Unlock()
		return
	}

	a.renderActiveView()
	go func() {
		params := map[string]any{"sessionId": sessionID, "cwd": workspace}
		if servers := a.mcpServersPayload(); len(servers) > 0 {
			params["mcpServers"] = servers
		}
		err := client.Call(a.ctx, "session/load", params, nil)

		tracker.mu.Lock()
		if err == nil {
			tracker.states[sessionID] = sessionHistoryLoaded
		} else {
			tracker.states[sessionID] = sessionHistoryUnloaded
		}
		tracker.mu.Unlock()

		if !a.clientIsCurrent(client) {
			return
		}
		if err != nil {
			a.setStatus("Session history failed · " + err.Error())
		}
		a.renderActiveView()
	}()
}

func formatUserTranscript(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return "\n\n> " + strings.ReplaceAll(text, "\n", "\n> ") + "\n\n"
}
