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
	mu      sync.Mutex
	states  map[string]sessionHistoryState
	staging map[string]*strings.Builder
}

var sessionHistoryTrackers sync.Map // map[*application]*sessionHistoryTracker

func sessionHistoryTrackerFor(a *application) *sessionHistoryTracker {
	if existing, ok := sessionHistoryTrackers.Load(a); ok {
		return existing.(*sessionHistoryTracker)
	}
	created := &sessionHistoryTracker{
		states:  make(map[string]sessionHistoryState),
		staging: make(map[string]*strings.Builder),
	}
	actual, _ := sessionHistoryTrackers.LoadOrStore(a, created)
	return actual.(*sessionHistoryTracker)
}

func sessionHistoryIsLoading(a *application, sessionID string) bool {
	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.states[sessionID] == sessionHistoryLoading
}

func markSessionHistoryLoaded(a *application, sessionID string) {
	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	tracker.states[sessionID] = sessionHistoryLoaded
	delete(tracker.staging, sessionID)
	tracker.mu.Unlock()
}

func resetLoadingSessionHistories(a *application) {
	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	for sessionID, state := range tracker.states {
		if state == sessionHistoryLoading {
			tracker.states[sessionID] = sessionHistoryUnloaded
			delete(tracker.staging, sessionID)
		}
	}
	tracker.mu.Unlock()
}

// stageSessionHistoryChunk diverts replayed message chunks away from the live
// transcript until session/load completes successfully. A failed or disconnected
// replay can therefore be retried without preserving or duplicating a partial history.
func stageSessionHistoryChunk(a *application, sessionID, text string) bool {
	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.states[sessionID] != sessionHistoryLoading {
		return false
	}
	buffer := tracker.staging[sessionID]
	if buffer == nil {
		buffer = &strings.Builder{}
		tracker.staging[sessionID] = buffer
	}
	buffer.WriteString(text)
	return true
}

func finishSessionHistoryLoad(a *application, sessionID string, loadErr error) {
	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	buffer := tracker.staging[sessionID]
	delete(tracker.staging, sessionID)
	if loadErr == nil {
		tracker.states[sessionID] = sessionHistoryLoaded
	} else {
		tracker.states[sessionID] = sessionHistoryUnloaded
	}
	tracker.mu.Unlock()

	if loadErr != nil {
		return
	}
	text := ""
	if buffer != nil {
		text = buffer.String()
	}
	a.mu.Lock()
	replacement := &strings.Builder{}
	replacement.WriteString(text)
	a.transcripts[sessionID] = replacement
	a.mu.Unlock()
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
	for _, session := range a.state.Sessions {
		if session.ID == sessionID {
			workspace = validWorkspacePath(session.Workspace)
			break
		}
	}
	a.mu.Unlock()

	tracker := sessionHistoryTrackerFor(a)
	tracker.mu.Lock()
	if state := tracker.states[sessionID]; state == sessionHistoryLoading || state == sessionHistoryLoaded {
		tracker.mu.Unlock()
		return
	}
	tracker.states[sessionID] = sessionHistoryLoading
	tracker.staging[sessionID] = &strings.Builder{}
	tracker.mu.Unlock()

	if workspace == "" {
		finishSessionHistoryLoad(a, sessionID, errSessionHistoryWorkspaceUnavailable)
		return
	}

	a.renderActiveView()
	go func() {
		params := map[string]any{"sessionId": sessionID, "cwd": workspace}
		if servers := a.mcpServersPayload(); len(servers) > 0 {
			params["mcpServers"] = servers
		}
		err := client.Call(a.ctx, "session/load", params, nil)
		finishSessionHistoryLoad(a, sessionID, err)

		if !a.clientIsCurrent(client) {
			return
		}
		if err != nil {
			a.setStatus("Session history failed · " + err.Error())
		}
		a.renderActiveView()
	}()
}

var errSessionHistoryWorkspaceUnavailable = &sessionHistoryLoadError{"workspace path unavailable"}

type sessionHistoryLoadError struct{ message string }

func (e *sessionHistoryLoadError) Error() string { return e.message }

func formatUserTranscript(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return "\n\n> " + strings.ReplaceAll(text, "\n", "\n> ") + "\n\n"
}
