//go:build desktop

package desktop

import (
	"strings"
	"sync"
	"time"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const memoryRefreshInterval = 3 * time.Second

type sessionMemoryResult struct {
	SessionID    string `json:"sessionId"`
	WorkspaceKey string `json:"workspaceKey"`
	Workspace    []struct {
		ID         string  `json:"id"`
		Scope      string  `json:"scope"`
		Kind       string  `json:"kind"`
		Key        string  `json:"key"`
		Value      string  `json:"value"`
		Confidence float64 `json:"confidence"`
		UsageCount uint64  `json:"usageCount"`
	} `json:"workspace"`
	Global []struct {
		ID         string  `json:"id"`
		Scope      string  `json:"scope"`
		Kind       string  `json:"kind"`
		Key        string  `json:"key"`
		Value      string  `json:"value"`
		Confidence float64 `json:"confidence"`
		UsageCount uint64  `json:"usageCount"`
	} `json:"global"`
}

type memoryRefreshTracker struct {
	mu       sync.Mutex
	last     map[string]time.Time
	inFlight map[string]bool
}

var memoryRefreshers sync.Map // map[*application]*memoryRefreshTracker

func memoryTrackerFor(a *application) *memoryRefreshTracker {
	if existing, ok := memoryRefreshers.Load(a); ok {
		return existing.(*memoryRefreshTracker)
	}
	created := &memoryRefreshTracker{last: make(map[string]time.Time), inFlight: make(map[string]bool)}
	actual, _ := memoryRefreshers.LoadOrStore(a, created)
	return actual.(*memoryRefreshTracker)
}

func (a *application) refreshSessionMemory(sessionID string, force bool) {
	sessionID = strings.TrimSpace(sessionID)
	client := a.currentClient()
	if sessionID == "" || client == nil {
		return
	}
	tracker := memoryTrackerFor(a)
	tracker.mu.Lock()
	if tracker.inFlight[sessionID] {
		tracker.mu.Unlock()
		return
	}
	if !force {
		if last := tracker.last[sessionID]; !last.IsZero() && time.Since(last) < memoryRefreshInterval {
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

		var result sessionMemoryResult
		if err := client.Call(a.ctx, "protonman/session/memory", map[string]any{"sessionId": sessionID}, &result); err != nil {
			return
		}
		if !a.clientIsCurrent(client) || strings.TrimSpace(result.SessionID) != sessionID {
			return
		}
		memory := desktopstate.MemoryState{WorkspaceKey: strings.TrimSpace(result.WorkspaceKey)}
		for _, item := range result.Workspace {
			memory.Workspace = append(memory.Workspace, desktopstate.MemoryEntryState{
				ID: strings.TrimSpace(item.ID), Scope: strings.TrimSpace(item.Scope), Kind: strings.TrimSpace(item.Kind),
				Key: strings.TrimSpace(item.Key), Value: strings.TrimSpace(item.Value), Confidence: item.Confidence, UsageCount: item.UsageCount,
			})
		}
		for _, item := range result.Global {
			memory.Global = append(memory.Global, desktopstate.MemoryEntryState{
				ID: strings.TrimSpace(item.ID), Scope: strings.TrimSpace(item.Scope), Kind: strings.TrimSpace(item.Kind),
				Key: strings.TrimSpace(item.Key), Value: strings.TrimSpace(item.Value), Confidence: item.Confidence, UsageCount: item.UsageCount,
			})
		}
		a.mu.Lock()
		a.state = desktopstate.Reduce(a.state, desktopstate.Event{Kind: desktopstate.EventSessionMemoryUpdated, SessionID: sessionID, Memory: memory})
		active := a.state.ActiveSessionID == sessionID
		a.mu.Unlock()
		if active {
			a.renderActiveView()
		}
	}()
}

func renderMemory(memory desktopstate.MemoryState) string {
	if len(memory.Workspace) == 0 && len(memory.Global) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("\n\n### Memory\n")
	if len(memory.Workspace) > 0 {
		out.WriteString("\n**Workspace**\n")
		for _, item := range memory.Workspace {
			writeMemoryEntry(&out, item)
		}
	}
	if len(memory.Global) > 0 {
		out.WriteString("\n**Global preferences**\n")
		for _, item := range memory.Global {
			writeMemoryEntry(&out, item)
		}
	}
	return out.String()
}

func writeMemoryEntry(out *strings.Builder, item desktopstate.MemoryEntryState) {
	key := strings.TrimSpace(item.Key)
	value := strings.TrimSpace(item.Value)
	if key == "" || value == "" {
		return
	}
	out.WriteString("\n- `")
	out.WriteString(strings.TrimSpace(item.Kind))
	out.WriteString("` **")
	out.WriteString(key)
	out.WriteString("**: ")
	out.WriteString(value)
}
