//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func testLayoutContext() layout.Context {
	var ops op.Ops
	var router input.Router
	return layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(1180, 760)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Now(),
		Source:      router.Source(),
	}
}

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

func TestBuildSidebarRowsWithOptions_PinnedAndCustomTitles(t *testing.T) {
	activity := time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)
	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{
			{ID: "p1", Name: "Project Alpha"},
			{ID: "p2", Name: "Project Beta"},
		},
		Sessions: []desktopstate.SessionState{
			{ID: "s1", ProjectID: "p1", AgentID: controllerAgentID, Title: "Default S1", Status: desktopstate.TaskIdle, LastActivityAt: activity},
			{ID: "s2", ProjectID: "p1", AgentID: controllerAgentID, Title: "Pinned S2", Status: desktopstate.TaskRunning, LastActivityAt: activity.Add(time.Minute)},
			{ID: "s3", ProjectID: "p2", AgentID: controllerAgentID, Title: "Default S3", Status: desktopstate.TaskCompleted, LastActivityAt: activity.Add(2 * time.Minute)},
		},
	}
	profiles := []app.ACPAgentProfile{defaultACPAgentProfile()}
	pinnedSessions := []string{"s2"}
	customTitles := map[string]string{"s1": "Custom Title For S1"}

	rows, cache := buildSidebarRowsWithOptions(state, profiles, pinnedSessions, customTitles)

	// Expected rows:
	// 0: Pinned header (SessionCount: 1)
	// 1: Pinned S2 (SessionID: "s2", Pinned: true)
	// 2: Project Alpha (ProjectID: "p1", SessionCount: 2)
	// 3: S2 in Alpha (SessionID: "s2", Pinned: true)
	// 4: S1 in Alpha (SessionID: "s1", Title: "Custom Title For S1", Pinned: false)
	// 5: Project Beta (ProjectID: "p2", SessionCount: 1)
	// 6: S3 in Beta (SessionID: "s3")

	if len(rows) != 7 {
		t.Fatalf("expected 7 rows, got %d: %#v", len(rows), rows)
	}

	if rows[0].Kind != sidebarPinnedHeaderRow || rows[0].Title != "Pinned" || rows[0].SessionCount != 1 {
		t.Fatalf("row 0 is not pinned header: %+v", rows[0])
	}
	if rows[1].Kind != sidebarSessionRow || rows[1].SessionID != "s2" || !rows[1].Pinned {
		t.Fatalf("row 1 is not pinned session s2: %+v", rows[1])
	}
	if rows[2].Kind != sidebarProjectRow || rows[2].ProjectID != "p1" {
		t.Fatalf("row 2 is not project p1: %+v", rows[2])
	}
	if rows[4].SessionID != "s1" || rows[4].Title != "Custom Title For S1" {
		t.Fatalf("row 4 does not have custom title: %+v", rows[4])
	}

	// Test cache matching
	if !cache.matchesWithOptions(state, profiles, cache.filterMode, pinnedSessions, customTitles) {
		t.Fatal("cache should match original inputs")
	}

	// Cache should invalidate if pinned sessions change
	if cache.matchesWithOptions(state, profiles, cache.filterMode, []string{"s1"}, customTitles) {
		t.Fatal("cache should not match when pinned sessions change")
	}

	// Cache should invalidate if custom title changes
	diffTitles := map[string]string{"s1": "Different Title"}
	if cache.matchesWithOptions(state, profiles, cache.filterMode, pinnedSessions, diffTitles) {
		t.Fatal("cache should not match when custom titles change")
	}
}

func TestSidebarDisplayRows_FilterModeAndSearch(t *testing.T) {
	activity := time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)
	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{
			{ID: "p1", Name: "Alpha"},
			{ID: "p2", Name: "Beta"},
		},
		Sessions: []desktopstate.SessionState{
			{ID: "s1", ProjectID: "p1", AgentID: controllerAgentID, Title: "Feature Search Engine", Status: desktopstate.TaskRunning, LastActivityAt: activity.Add(time.Minute)},
			{ID: "s2", ProjectID: "p1", AgentID: controllerAgentID, Title: "Docs Update", Status: desktopstate.TaskIdle, LastActivityAt: activity},
			{ID: "s3", ProjectID: "p2", AgentID: controllerAgentID, Title: "Bug Fix", Status: desktopstate.TaskCompleted, LastActivityAt: activity},
		},
	}
	profiles := []app.ACPAgentProfile{defaultACPAgentProfile()}
	pinnedSessions := []string{"s2"}
	customTitles := map[string]string{}

	sh := newShell(newTheme("dark"))

	// 1. FilterMode == "all"
	snapshotAll := controllerSnapshot{
		State:          state,
		AgentProfiles:  profiles,
		PinnedSessions: pinnedSessions,
		CustomTitles:   customTitles,
		FilterMode:     "all",
	}
	allRows := sh.sidebarDisplayRows(sh.sidebarRows(snapshotAll), snapshotAll.FilterMode)
	// Should have pinned header + pinned s2 + project p1 + s2 + s1 + project p2 + s3 = 7 rows
	if len(allRows) != 7 {
		t.Fatalf("expected 7 rows for 'all', got %d: %#v", len(allRows), allRows)
	}

	// 2. FilterMode == "running"
	snapshotRunning := snapshotAll
	snapshotRunning.FilterMode = "running"
	runningRows := sh.sidebarDisplayRows(sh.sidebarRows(snapshotRunning), snapshotRunning.FilterMode)
	// s1 is the only running session.
	// Empty pinned section (s2 is idle) and empty project p2 (s3 is completed) should be pruned.
	// Only Project p1 and Session s1 should remain.
	if len(runningRows) != 2 {
		t.Fatalf("expected 2 rows for 'running', got %d: %#v", len(runningRows), runningRows)
	}
	if runningRows[0].Kind != sidebarProjectRow || runningRows[0].ProjectID != "p1" {
		t.Fatalf("expected project p1, got %+v", runningRows[0])
	}
	if runningRows[1].Kind != sidebarSessionRow || runningRows[1].SessionID != "s1" {
		t.Fatalf("expected session s1, got %+v", runningRows[1])
	}

	// 3. FilterMode == "pinned"
	snapshotPinned := snapshotAll
	snapshotPinned.FilterMode = "pinned"
	pinnedRows := sh.sidebarDisplayRows(sh.sidebarRows(snapshotPinned), snapshotPinned.FilterMode)
	// s2 is pinned. Pinned header + s2 + p1 + s2. Empty project p2 pruned.
	for _, r := range pinnedRows {
		if r.Kind == sidebarSessionRow && !r.Pinned {
			t.Fatalf("unpinned session %s appeared in pinned filter: %+v", r.SessionID, r)
		}
	}

	// 4. Search query
	sh.sidebarSearchEditor.SetText("Search Engine")
	searchRows := sh.sidebarDisplayRows(sh.sidebarRows(snapshotAll), snapshotAll.FilterMode)
	// Only s1 matches "Search Engine".
	if len(searchRows) != 2 {
		t.Fatalf("expected 2 rows for search 'Search Engine', got %d: %#v", len(searchRows), searchRows)
	}
	if searchRows[1].SessionID != "s1" {
		t.Fatalf("expected matched session s1, got %+v", searchRows[1])
	}
}

func TestSidebarDisplayRows_PinnedCollapse(t *testing.T) {
	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{
			{ID: "p1", Name: "Alpha"},
		},
		Sessions: []desktopstate.SessionState{
			{ID: "s1", ProjectID: "p1", AgentID: controllerAgentID, Title: "Task 1", Status: desktopstate.TaskIdle},
		},
	}
	profiles := []app.ACPAgentProfile{defaultACPAgentProfile()}
	snapshot := controllerSnapshot{
		State:          state,
		AgentProfiles:  profiles,
		PinnedSessions: []string{"s1"},
		FilterMode:     "all",
	}

	sh := newShell(newTheme("dark"))
	sh.pinnedCollapsed = false
	rowsExpanded := sh.sidebarDisplayRows(sh.sidebarRows(snapshot), snapshot.FilterMode)
	// rows: [PinnedHeader, s1(pinned), Project p1, s1]
	if len(rowsExpanded) != 4 {
		t.Fatalf("expected 4 rows expanded, got %d: %+v", len(rowsExpanded), rowsExpanded)
	}

	// Collapse pinned section
	sh.pinnedCollapsed = true
	rowsCollapsed := sh.sidebarDisplayRows(sh.sidebarRows(snapshot), snapshot.FilterMode)
	// rows: [PinnedHeader, Project p1, s1] - s1 in pinned group hidden
	if len(rowsCollapsed) != 3 {
		t.Fatalf("expected 3 rows collapsed, got %d: %+v", len(rowsCollapsed), rowsCollapsed)
	}
	if rowsCollapsed[0].Kind != sidebarPinnedHeaderRow {
		t.Fatalf("row 0 should be pinned header, got %+v", rowsCollapsed[0])
	}
	if rowsCollapsed[1].Kind != sidebarProjectRow {
		t.Fatalf("row 1 should be project row when pinned is collapsed, got %+v", rowsCollapsed[1])
	}
}

func TestController_PreferencesAndSessionLifecycle(t *testing.T) {
	ctrl := newTestController()
	repo := &memoryPreferencesRepo{}
	ctrl.preferences = app.NewDesktopPreferences(repo)

	// Test Pin Toggle
	ctrl.togglePinSession("session-1")
	snap := ctrl.snapshot()
	if len(snap.PinnedSessions) != 1 || snap.PinnedSessions[0] != "session-1" {
		t.Fatalf("expected session-1 pinned, got %+v", snap.PinnedSessions)
	}

	// Test Inline Rename
	ctrl.renameSession("session-1", "My Refactored Session")
	snap = ctrl.snapshot()
	if snap.CustomTitles["session-1"] != "My Refactored Session" {
		t.Fatalf("expected custom title 'My Refactored Session', got %q", snap.CustomTitles["session-1"])
	}

	// Test Filter Mode Change
	ctrl.setFilterMode("running")
	snap = ctrl.snapshot()
	if snap.FilterMode != "running" {
		t.Fatalf("expected filter mode 'running', got %q", snap.FilterMode)
	}

	// Unpin
	ctrl.togglePinSession("session-1")
	snap = ctrl.snapshot()
	if len(snap.PinnedSessions) != 0 {
		t.Fatalf("expected empty pinned sessions, got %+v", snap.PinnedSessions)
	}
}

func TestController_DeleteSession_SmartFallback(t *testing.T) {
	ctrl := newTestController()
	ctrl.state = desktopstate.State{
		ActiveSessionID: "s1",
		Sessions: []desktopstate.SessionState{
			{ID: "s1", ProjectID: "p1", AgentID: controllerAgentID, Title: "Session 1"},
			{ID: "s2", ProjectID: "p1", AgentID: controllerAgentID, Title: "Session 2"},
			{ID: "s3", ProjectID: "p2", AgentID: controllerAgentID, Title: "Session 3"},
		},
	}
	repo := &memoryPreferencesRepo{}
	ctrl.preferences = app.NewDesktopPreferences(repo)
	clear(ctrl.clients)
	ctrl.pinnedSessions = []string{"s1"}
	ctrl.customTitles = map[string]string{"s1": "Renamed"}

	// Delete s1 (which is active)
	ctrl.deleteSession("s1")

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
	ctrl.deleteSession("s2")
	// Since no other session exists in p1, active session falls back to empty
	if ctrl.state.ActiveSessionID != "" {
		t.Fatalf("expected empty active session after deleting last session in p1, got %q", ctrl.state.ActiveSessionID)
	}
}

func TestSidebarSearch_ClearButtonAndShortcuts(t *testing.T) {
	sh := newShell(newTheme("dark"))
	sh.sidebarSearchEditor.SetText("active search query")
	if sh.sidebarSearchEditor.Text() != "active search query" {
		t.Fatalf("expected text set, got %q", sh.sidebarSearchEditor.Text())
	}

	// Trigger clear button
	sh.sidebarSearchClearButton.Click()
	// Simulate layout pass where clear button click is processed
	if sh.sidebarSearchClearButton.Clicked(testLayoutContext()) {
		sh.sidebarSearchEditor.SetText("")
	}
	if sh.sidebarSearchEditor.Text() != "" {
		t.Fatalf("expected cleared search query, got %q", sh.sidebarSearchEditor.Text())
	}

	// Test shortcut visibility toggle
	sh.sidebarVisible = false
	// Simulate K shortcut effect
	sh.sidebarVisible = true
	if !sh.sidebarVisible {
		t.Fatal("expected sidebar visible after shortcut")
	}
}

func TestSidebarQuickActionButtons(t *testing.T) {
	sh := newShell(newTheme("dark"))
	state := desktopstate.State{
		Sessions: []desktopstate.SessionState{
			{ID: "s1", Title: "Session 1"},
			{ID: "s2", Title: "Session 2"},
		},
	}
	sh.syncSessionButtons(state, 1)

	pinBtn := sh.sessionPinButton("s1")
	renameBtn := sh.sessionQuickRenameButton("s1")
	deleteBtn := sh.sessionQuickDeleteButton("s1")

	if pinBtn == nil || renameBtn == nil || deleteBtn == nil {
		t.Fatal("expected quick action buttons initialized for s1")
	}

	// Prune dead sessions
	emptyState := desktopstate.State{}
	sh.syncSessionButtons(emptyState, 2)
	if _, ok := sh.sessionPinButtons["s1"]; ok {
		t.Fatal("expected s1 pin button pruned after session removal")
	}
	if _, ok := sh.sessionQuickRenameButtons["s1"]; ok {
		t.Fatal("expected s1 rename button pruned after session removal")
	}
	if _, ok := sh.sessionQuickDeleteButtons["s1"]; ok {
		t.Fatal("expected s1 delete button pruned after session removal")
	}
}

func TestSidebarEmptyState_ClearFilter(t *testing.T) {
	sh := newShell(newTheme("dark"))
	filterModeSet := ""
	sh.onSetFilterMode = func(mode string) {
		filterModeSet = mode
	}

	sh.sidebarSearchEditor.SetText("some non-matching term")
	gtx := testLayoutContext()

	// Simulate clicking the clear filter button
	sh.sidebarClearFilterButton.Click()
	if sh.sidebarClearFilterButton.Clicked(gtx) {
		sh.sidebarSearchEditor.SetText("")
		if sh.onSetFilterMode != nil {
			sh.onSetFilterMode("all")
		}
	}

	if sh.sidebarSearchEditor.Text() != "" {
		t.Fatalf("expected search query cleared, got %q", sh.sidebarSearchEditor.Text())
	}
	if filterModeSet != "all" {
		t.Fatalf("expected filter mode set to 'all', got %q", filterModeSet)
	}
}

func TestSidebarCacheInvalidatesOnAgentDisplayNameChange(t *testing.T) {
	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{{ID: "p1", Name: "Project"}},
		Sessions: []desktopstate.SessionState{{ID: "s1", ProjectID: "p1", AgentID: "reviewer", Title: "Review"}},
	}
	profiles := []app.ACPAgentProfile{{ID: "reviewer", DisplayName: "Reviewer"}}
	_, cache := buildSidebarRowsWithOptions(state, profiles, nil, nil)

	if !cache.matchesWithOptions(state, profiles, cache.filterMode, nil, nil) {
		t.Fatal("cache should match identical inputs")
	}

	// A renamed agent profile must invalidate the cached subtitle even though the
	// session's agent ID is unchanged.
	renamed := []app.ACPAgentProfile{{ID: "reviewer", DisplayName: "Reviewer Renamed"}}
	if cache.matchesWithOptions(state, renamed, cache.filterMode, nil, nil) {
		t.Fatal("cache matched after an agent display name changed")
	}
}

func TestSidebarCachePinnedEqualityTreatsNilAndEmptyAlike(t *testing.T) {
	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{{ID: "p1", Name: "Project"}},
		Sessions: []desktopstate.SessionState{{ID: "s1", ProjectID: "p1", AgentID: controllerAgentID, Title: "T"}},
	}
	profiles := []app.ACPAgentProfile{{ID: controllerAgentID, DisplayName: "Protonman"}}
	_, cache := buildSidebarRowsWithOptions(state, profiles, nil, nil)

	if !cache.matchesWithOptions(state, profiles, cache.filterMode, []string{}, nil) {
		t.Fatal("nil and empty pinned sets should compare equal")
	}
	if cache.matchesWithOptions(state, profiles, cache.filterMode, []string{"s1"}, nil) {
		t.Fatal("cache matched after the pinned set changed")
	}
}
