//go:build desktop || desktop_gio

package controller

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestController_DeleteSession_SmartFallback(t *testing.T) {
	ctrl := newTestController()
	ctrl.state = desktopstate.State{
		ActiveSessionID: "s1",
		Sessions: []desktopstate.SessionState{
			{ID: "s1", ProjectID: "p1", AgentID: ProtonmanAgentID, Title: "Session 1"},
			{ID: "s2", ProjectID: "p1", AgentID: ProtonmanAgentID, Title: "Session 2"},
			{ID: "s3", ProjectID: "p2", AgentID: ProtonmanAgentID, Title: "Session 3"},
		},
	}
	repo := &memoryPreferencesRepo{}
	ctrl.preferences = app.NewDesktopPreferences(repo)
	clear(ctrl.clients)
	ctrl.pinnedSessions = []string{"s1"}
	ctrl.customTitles = map[string]string{"s1": "Renamed"}

	// Delete s1 (which is active)
	ctrl.DeleteSession("s1")

	// s2 should now be selected as fallback in p1
	if ctrl.state.ActiveSessionID != "s2" {
		t.Fatalf("expected active session s2, got %q", ctrl.state.ActiveSessionID)
	}
	if len(ctrl.state.Sessions) != 2 {
		t.Fatalf("expected 2 sessions remaining, got %d", len(ctrl.state.Sessions))
	}
	// Pinned and custom titles should have been cleaned up
	if len(ctrl.pinnedSessions) != 0 {
		t.Fatalf("expected pinned sessions to be cleaned, got %+v", ctrl.pinnedSessions)
	}
	if _, exists := ctrl.customTitles["s1"]; exists {
		t.Fatalf("expected s1 custom title to be removed")
	}

	// Delete s2 (last session in p1)
	ctrl.DeleteSession("s2")
	// Since no other session exists in p1, active session falls back to empty
	if ctrl.state.ActiveSessionID != "" {
		t.Fatalf("expected empty active session after deleting last session in p1, got %q", ctrl.state.ActiveSessionID)
	}
}

func TestController_PreferencesAndSessionLifecycle(t *testing.T) {
	ctrl := newTestController()
	repo := &memoryPreferencesRepo{}
	ctrl.preferences = app.NewDesktopPreferences(repo)

	// Test Pin Toggle
	ctrl.TogglePinSession("session-1")
	snap := ctrl.Snapshot()
	if len(snap.PinnedSessions) != 1 || snap.PinnedSessions[0] != "session-1" {
		t.Fatalf("expected session-1 pinned, got %+v", snap.PinnedSessions)
	}

	// Test Inline Rename
	ctrl.RenameSession("session-1", "My Refactored Session")
	snap = ctrl.Snapshot()
	if snap.CustomTitles["session-1"] != "My Refactored Session" {
		t.Fatalf("expected custom title 'My Refactored Session', got %q", snap.CustomTitles["session-1"])
	}

	// Test Filter Mode Change
	ctrl.SetFilterMode("running")
	snap = ctrl.Snapshot()
	if snap.FilterMode != "running" {
		t.Fatalf("expected filter mode 'running', got %q", snap.FilterMode)
	}

	// Unpin
	ctrl.TogglePinSession("session-1")
	snap = ctrl.Snapshot()
	if len(snap.PinnedSessions) != 0 {
		t.Fatalf("expected empty pinned sessions, got %+v", snap.PinnedSessions)
	}
}

// memoryPreferencesRepo is an in-memory app.DesktopPreferencesRepository used
// to assert what the coordinator persists without touching the real store.
type memoryPreferencesRepo struct {
	state app.DesktopPreferencesState
}

func (m *memoryPreferencesRepo) Load(context.Context) (app.DesktopPreferencesState, error) {
	return m.state, nil
}

func (m *memoryPreferencesRepo) Save(_ context.Context, state app.DesktopPreferencesState) error {
	m.state = state
	return nil
}
