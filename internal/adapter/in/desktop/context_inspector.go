//go:build desktop

package desktop

import (
	"strings"
	"sync"
	"time"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

const contextRefreshInterval = 750 * time.Millisecond

type sessionContextResult struct {
	SessionID string `json:"sessionId"`
	Goal      string `json:"goal"`
	Todo      struct {
		Revision uint64 `json:"revision"`
		Items    []struct {
			ID     string `json:"id"`
			Text   string `json:"text"`
			Status string `json:"status"`
		} `json:"items"`
	} `json:"todo"`
}

type contextRefreshTracker struct {
	mu       sync.Mutex
	last     map[string]time.Time
	inFlight map[string]bool
}

var contextRefreshers sync.Map // map[*application]*contextRefreshTracker

func contextTrackerFor(a *application) *contextRefreshTracker {
	if existing, ok := contextRefreshers.Load(a); ok {
		return existing.(*contextRefreshTracker)
	}
	created := &contextRefreshTracker{
		last:     make(map[string]time.Time),
		inFlight: make(map[string]bool),
	}
	actual, _ := contextRefreshers.LoadOrStore(a, created)
	return actual.(*contextRefreshTracker)
}

func (a *application) refreshSessionContext(sessionID string, force bool) {
	sessionID = strings.TrimSpace(sessionID)
	client := a.clientForSession(sessionID)
	if sessionID == "" || client == nil || !a.protonmanExtensionsAvailable() {
		return
	}

	tracker := contextTrackerFor(a)
	tracker.mu.Lock()
	if tracker.inFlight[sessionID] {
		tracker.mu.Unlock()
		return
	}
	if !force {
		if last := tracker.last[sessionID]; !last.IsZero() && time.Since(last) < contextRefreshInterval {
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

		var result sessionContextResult
		if err := client.Call(a.ctx, "protonman/session/context", map[string]any{"sessionId": sessionID}, &result); err != nil {
			return
		}
		if !a.clientIsCurrent(client) || strings.TrimSpace(result.SessionID) != sessionID {
			return
		}

		items := make([]desktopstate.TodoItemState, 0, len(result.Todo.Items))
		for _, item := range result.Todo.Items {
			items = append(items, desktopstate.TodoItemState{
				ID:     strings.TrimSpace(item.ID),
				Text:   strings.TrimSpace(item.Text),
				Status: strings.TrimSpace(item.Status),
			})
		}

		a.mu.Lock()
		a.state = desktopstate.Reduce(a.state, desktopstate.Event{
			Kind:      desktopstate.EventSessionContextUpdated,
			SessionID: sessionID,
			Context: desktopstate.SessionContextState{
				Goal: strings.TrimSpace(result.Goal),
				Todo: desktopstate.TodoState{Revision: result.Todo.Revision, Items: items},
			},
		})
		active := a.state.ActiveSessionID == sessionID
		a.mu.Unlock()
		if active {
			a.renderActiveView()
		}
	}()
}

func renderSessionContext(context desktopstate.SessionContextState) string {
	goal := strings.TrimSpace(context.Goal)
	items := context.Todo.Items
	if goal == "" && len(items) == 0 {
		return ""
	}

	var out strings.Builder
	out.WriteString("\n\n### Context\n")
	if goal != "" {
		out.WriteString("\n**Goal**\n\n")
		out.WriteString(goal)
		out.WriteString("\n")
	}
	if len(items) > 0 {
		out.WriteString("\n**Todo")
		if context.Todo.Revision > 0 {
			out.WriteString(" · revision ")
			out.WriteString(uintToString(context.Todo.Revision))
		}
		out.WriteString("**\n")
		for _, item := range items {
			text := strings.TrimSpace(item.Text)
			if text == "" {
				continue
			}
			out.WriteString("\n")
			out.WriteString(todoMarker(item.Status))
			out.WriteString(" ")
			out.WriteString(text)
		}
		out.WriteString("\n")
	}
	return out.String()
}

func todoMarker(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		return "✓"
	case "in_progress":
		return "◐"
	default:
		return "○"
	}
}

func uintToString(value uint64) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	i := len(buffer)
	for value > 0 {
		i--
		buffer[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[i:])
}
