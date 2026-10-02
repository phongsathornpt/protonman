//go:build desktop || desktop_gio

package sidebar

import (
	"fmt"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestComponentProjectsAndCachesSidebarRows(t *testing.T) {
	activity := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	model := Model{
		State: desktopstate.State{
			Projects: []desktopstate.ProjectState{{ID: "workspace", Name: "Workspace"}},
			Sessions: []desktopstate.SessionState{
				{ID: "older", ProjectID: "workspace", AgentID: "protonman", Title: "Older", Status: desktopstate.TaskIdle, LastActivityAt: activity},
				{ID: "newer", ProjectID: "workspace", AgentID: "protonman", Title: "Newer", Status: desktopstate.TaskRunning, LastActivityAt: activity.Add(time.Minute)},
			},
		},
		AgentProfiles: []app.ACPAgentProfile{{ID: "protonman", DisplayName: "Protonman"}},
		Pinned:        []string{"older"},
		CustomTitles:  map[string]string{"older": "Pinned title"},
		FilterMode:    "all",
		Revision:      1,
	}
	component := New()
	first := component.Rows(model)
	second := component.Rows(model)
	if len(first) != 5 || &first[0] != &second[0] {
		t.Fatalf("rows = %#v; unchanged model should reuse the cached projection", first)
	}
	if first[0].Kind != PinnedHeaderRow || first[1].Title != "Pinned title" || first[2].Kind != ProjectRow || first[3].SessionID != "newer" {
		t.Fatalf("unexpected sidebar row projection: %#v", first)
	}

	model.State.Sessions[1].Status = desktopstate.TaskFailed
	model.Revision++
	updated := component.Rows(model)
	if &first[0] == &updated[0] || updated[3].Status != "Failed" {
		t.Fatalf("visible status change did not rebuild rows: %#v", updated)
	}
}

func TestComponentDisplayCacheTracksFiltersAndCollapse(t *testing.T) {
	component := New()
	rows := []Row{
		{Kind: ProjectRow, ProjectID: "p", Title: "Project"},
		{Kind: SessionRow, ProjectID: "p", SessionID: "running", Title: "Running", Status: "Running"},
		{Kind: SessionRow, ProjectID: "p", SessionID: "idle", Title: "Idle", Status: "Idle"},
	}
	all := component.DisplayRows(rows, "", "all", 4)
	if cached := component.DisplayRows(rows, "", "all", 4); &all[0] != &cached[0] {
		t.Fatal("unchanged presentation did not reuse display rows")
	}
	running := component.DisplayRows(rows, "", "running", 4)
	if len(running) != 2 || running[1].SessionID != "running" {
		t.Fatalf("running filter rows = %#v", running)
	}
	component.ToggleProject("p")
	collapsed := component.DisplayRows(rows, "", "all", 4)
	if len(collapsed) != 1 || collapsed[0].Kind != ProjectRow {
		t.Fatalf("collapsed rows = %#v", collapsed)
	}
}

func TestSyncSessionButtonsKeepsACPIdentitiesSeparateAndPrunesRemoved(t *testing.T) {
	component := New()
	state := desktopstate.State{Sessions: []desktopstate.SessionState{
		{ID: "shared", AgentID: "protonman"},
		{ID: "shared", AgentID: "reviewer"},
	}}
	component.SyncSessionButtons(state, 1)
	protonKey := SessionWidgetKey("shared", "protonman")
	reviewerKey := SessionWidgetKey("shared", "reviewer")
	if component.SessionButton(protonKey) == component.SessionButton(reviewerKey) {
		t.Fatal("same ACP session ID shared its sidebar clickable")
	}
	component.SyncSessionButtons(desktopstate.State{}, 2)
	if component.HasSessionButton(protonKey) || component.HasPinButton(protonKey) {
		t.Fatal("removed session retained sidebar widget state")
	}
}

// Rows must lead with the project header, order sessions by most recent
// activity, keep the default-agent session without a timestamp last, and hide
// the idle status badge. The projection cache must also invalidate when a
// session's activity timestamp changes, otherwise a session that just ran would
// stay pinned in the wrong position until the next unrelated state event.
func TestRowsOrderSessionsByRecencyAndHideIdleBadges(t *testing.T) {
	activity := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	model := Model{
		State: desktopstate.State{
			Projects: []desktopstate.ProjectState{{ID: "project", Name: "Project"}},
			Sessions: []desktopstate.SessionState{
				{ID: "older", ProjectID: "project", AgentID: "protonman", Title: "hi", Status: desktopstate.TaskIdle, LastActivityAt: activity},
				{ID: "newer", ProjectID: "project", AgentID: "protonman", Title: "hi", Status: desktopstate.TaskIdle, LastActivityAt: activity.Add(time.Hour)},
				{ID: "unknown", ProjectID: "project", AgentID: "protonman", Title: "proton", Status: desktopstate.TaskIdle},
			},
		},
		AgentProfiles: []app.ACPAgentProfile{{ID: "protonman", DisplayName: "Protonman"}},
	}

	rows := BuildRows(model).Rows()
	if len(rows) != 4 || rows[0].SessionCount != 3 {
		t.Fatalf("rows = %+v, want a project header with SessionCount 3 followed by three sessions", rows)
	}
	if rows[1].SessionID != "newer" || rows[2].SessionID != "older" || rows[3].SessionID != "unknown" {
		t.Fatalf("session order = %q, %q, %q; want newest activity first", rows[1].SessionID, rows[2].SessionID, rows[3].SessionID)
	}
	if got := StatusLabel("idle"); got != "" {
		t.Fatalf("idle status label = %q, want hidden", got)
	}
	if got := StatusLabel("waiting_permission"); got != "Approval" {
		t.Fatalf("permission status label = %q, want Approval", got)
	}
	now := activity.Add(3 * time.Minute)
	if got := SessionSubtitle(rows[2], now); got != "3m ago" {
		t.Fatalf("activity subtitle = %q, want 3m ago", got)
	}
	if got := SessionSubtitle(rows[3], now); got != "" {
		t.Fatalf("session without a timestamp should render no subtitle, got %q", got)
	}

	// A changed activity timestamp must invalidate the projection cache.
	model.State.Sessions[0].LastActivityAt = activity.Add(2 * time.Hour)
	cache := BuildRows(model)
	model.State.Sessions[0].LastActivityAt = activity.Add(4 * time.Hour)
	if cache.Matches(model) {
		t.Fatal("rows cache matched after a session activity timestamp changed")
	}
}

func indexOfSession(rows []Row, sessionID string) int {
	for i, row := range rows {
		if row.Kind == SessionRow && row.SessionID == sessionID {
			return i
		}
	}
	return -1
}

// The session the user is working in gets a fresh LastActivityAt on every
// prompt, which would otherwise throw it to the top of its project group and
// shuffle the rows under the cursor. Recency ordering applies to every other
// row, but the active session keeps the position it already occupied.
func TestActiveSessionKeepsPositionWhenItsActivityUpdates(t *testing.T) {
	activity := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	model := Model{
		State: desktopstate.State{
			Projects: []desktopstate.ProjectState{{ID: "workspace", Name: "Workspace"}},
			Sessions: []desktopstate.SessionState{
				{ID: "first", ProjectID: "workspace", AgentID: "protonman", Title: "First", LastActivityAt: activity.Add(2 * time.Minute)},
				{ID: "second", ProjectID: "workspace", AgentID: "protonman", Title: "Second", LastActivityAt: activity.Add(time.Minute)},
				{ID: "active", ProjectID: "workspace", AgentID: "protonman", Title: "Active", LastActivityAt: activity},
			},
			ActiveSessionID: "active",
			ActiveAgentID:   "protonman",
		},
		AgentProfiles: []app.ACPAgentProfile{{ID: "protonman", DisplayName: "Protonman"}},
		FilterMode:    "all",
		Revision:      1,
	}
	component := New()

	before := indexOfSession(component.Rows(model), "active")
	if before != 3 {
		t.Fatalf("active session index = %d, want 3 (project header plus three sessions)", before)
	}

	// Sending a prompt in the active session makes it the most recent row.
	model.State.Sessions[2].LastActivityAt = activity.Add(time.Hour)
	model.Revision++
	after := indexOfSession(component.Rows(model), "active")
	if after != before {
		t.Fatalf("active session moved from index %d to %d; it must keep its sidebar position", before, after)
	}
}

// Stabilising the active session must not freeze the whole list: the other
// sessions still re-sort by recency around it.
func TestNonActiveSessionsStillSortByRecency(t *testing.T) {
	activity := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	model := Model{
		State: desktopstate.State{
			Projects: []desktopstate.ProjectState{{ID: "workspace", Name: "Workspace"}},
			Sessions: []desktopstate.SessionState{
				{ID: "active", ProjectID: "workspace", AgentID: "protonman", Title: "Active", LastActivityAt: activity},
				{ID: "b", ProjectID: "workspace", AgentID: "protonman", Title: "B", LastActivityAt: activity.Add(time.Minute)},
				{ID: "c", ProjectID: "workspace", AgentID: "protonman", Title: "C", LastActivityAt: activity.Add(2 * time.Minute)},
			},
			ActiveSessionID: "active",
			ActiveAgentID:   "protonman",
		},
		AgentProfiles: []app.ACPAgentProfile{{ID: "protonman", DisplayName: "Protonman"}},
		FilterMode:    "all",
		Revision:      1,
	}
	component := New()
	if got := indexOfSession(component.Rows(model), "active"); got != 3 {
		t.Fatalf("active session index = %d, want 3", got)
	}

	model.State.Sessions[1].LastActivityAt = activity.Add(3 * time.Hour)
	model.Revision++
	rows := component.Rows(model)

	if got := indexOfSession(rows, "active"); got != 3 {
		t.Fatalf("active session index = %d, want 3; it must not move", got)
	}
	b, c := indexOfSession(rows, "b"), indexOfSession(rows, "c")
	if b < 0 || c < 0 || b > c {
		t.Fatalf("session order = %#v, want b re-sorted ahead of c", rows)
	}
}

func TestDisplayRowsSearchesInsideCollapsedProjects(t *testing.T) {
	component := New()
	rows := []Row{
		{Kind: ProjectRow, ProjectID: "p1", Title: "Project Alpha"},
		{Kind: SessionRow, ProjectID: "p1", SessionID: "s1", Title: "Fix authentication bug"},
		{Kind: SessionRow, ProjectID: "p1", SessionID: "s2", Title: "Database migration"},
	}
	component.ToggleProject("p1")
	if !component.ProjectCollapsed("p1") {
		t.Fatal("expected project p1 to be collapsed")
	}

	// While collapsed and no search query, session rows should be hidden.
	resting := component.DisplayRows(rows, "", "all", 1)
	if len(resting) != 1 || resting[0].Kind != ProjectRow {
		t.Fatalf("collapsed rows without search = %#v, want project row only", resting)
	}

	// While searching, matching session must be revealed even if project is collapsed.
	matched := component.DisplayRows(rows, "migration", "all", 1)
	if len(matched) != 2 || matched[0].Kind != ProjectRow || matched[1].SessionID != "s2" {
		t.Fatalf("search inside collapsed project = %#v, want project header and matching session s2", matched)
	}
}

func TestDisplayRowsPinnedFilterDeduplicatesSessions(t *testing.T) {
	component := New()
	rows := []Row{
		{Kind: PinnedHeaderRow, Title: "Pinned", SessionCount: 1},
		{Kind: SessionRow, ProjectID: "p1", SessionID: "s1", Title: "Pinned Session", Pinned: true},
		{Kind: ProjectRow, ProjectID: "p1", Title: "Project 1", SessionCount: 2},
		{Kind: SessionRow, ProjectID: "p1", SessionID: "s1", Title: "Pinned Session", Pinned: true},
		{Kind: SessionRow, ProjectID: "p1", SessionID: "s2", Title: "Normal Session", Pinned: false},
	}

	pinnedDisplay := component.DisplayRows(rows, "", "pinned", 1)
	if len(pinnedDisplay) != 2 {
		t.Fatalf("pinned display rows = %d, want exactly 2 (Pinned header + 1 session): %#v", len(pinnedDisplay), pinnedDisplay)
	}
	if pinnedDisplay[0].Kind != PinnedHeaderRow || pinnedDisplay[1].SessionID != "s1" {
		t.Fatalf("pinned display content = %#v, want Pinned header and s1", pinnedDisplay)
	}
}

func TestPinnedSessionScopedWidgetKeyPreventsPointerCollision(t *testing.T) {
	model := Model{
		State: desktopstate.State{
			Projects: []desktopstate.ProjectState{{ID: "p1", Name: "Project 1"}},
			Sessions: []desktopstate.SessionState{
				{ID: "s1", ProjectID: "p1", AgentID: "protonman", Title: "Session 1"},
			},
		},
		Pinned: []string{"s1"},
	}
	cache := BuildRows(model)
	rows := cache.Rows()
	// Row 0 is PinnedHeaderRow, Row 1 is pinned SessionRow, Row 2 is ProjectRow, Row 3 is project SessionRow
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	pinnedRow := rows[1]
	projectRow := rows[3]
	if pinnedRow.SessionID != "s1" || projectRow.SessionID != "s1" {
		t.Fatalf("unexpected session IDs: pinned=%q, project=%q", pinnedRow.SessionID, projectRow.SessionID)
	}
	if pinnedRow.SessionKey == projectRow.SessionKey {
		t.Fatalf("pinned row and project row must have distinct SessionKey to prevent widget collision: %q vs %q", pinnedRow.SessionKey, projectRow.SessionKey)
	}
	if pinnedRow.SessionKey != "pinned:"+projectRow.SessionKey {
		t.Fatalf("pinned SessionKey = %q, want pinned:%s", pinnedRow.SessionKey, projectRow.SessionKey)
	}
}

func TestRowsCacheMatchesTracksActiveSessionAndSortTimestamp(t *testing.T) {
	activity := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	modelA := Model{
		State: desktopstate.State{
			Projects: []desktopstate.ProjectState{{ID: "p1", Name: "Project 1"}},
			Sessions: []desktopstate.SessionState{
				{ID: "s1", ProjectID: "p1", AgentID: "protonman", Title: "One", LastActivityAt: activity},
				{ID: "s2", ProjectID: "p1", AgentID: "protonman", Title: "Two", LastActivityAt: activity.Add(time.Minute)},
			},
			ActiveSessionID: "s1",
			ActiveAgentID:   "protonman",
		},
	}
	cache := BuildRows(modelA)
	if !cache.Matches(modelA) {
		t.Fatal("cache should match identical modelA")
	}

	// Changing ActiveSessionID must invalidate cache
	modelB := modelA
	modelB.State.ActiveSessionID = "s2"
	if cache.Matches(modelB) {
		t.Fatal("cache should NOT match when ActiveSessionID changes")
	}

	// Changing ActiveSortAt must invalidate cache
	modelC := modelA
	sortTime := activity.Add(5 * time.Minute)
	modelC.ActiveSortAt = &sortTime
	if cache.Matches(modelC) {
		t.Fatal("cache should NOT match when ActiveSortAt changes")
	}
}

func TestSortSessionsDeterministicTieBreaker(t *testing.T) {
	// Sessions with equal timestamps must sort deterministically by SessionID
	model := Model{
		State: desktopstate.State{
			Projects: []desktopstate.ProjectState{{ID: "p1", Name: "Project"}},
			Sessions: []desktopstate.SessionState{
				{ID: "beta", ProjectID: "p1", AgentID: "protonman", Title: "Beta"},
				{ID: "alpha", ProjectID: "p1", AgentID: "protonman", Title: "Alpha"},
				{ID: "gamma", ProjectID: "p1", AgentID: "protonman", Title: "Gamma"},
			},
		},
	}
	cache := BuildRows(model)
	rows := cache.Rows()
	// Row 0 is ProjectRow, then sessions
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	if rows[1].SessionID != "alpha" || rows[2].SessionID != "beta" || rows[3].SessionID != "gamma" {
		t.Fatalf("sessions not sorted deterministically by ID: %s, %s, %s", rows[1].SessionID, rows[2].SessionID, rows[3].SessionID)
	}
}

func TestEmptySessionTitleFallsBackToUntitled(t *testing.T) {
	model := Model{
		State: desktopstate.State{
			Projects: []desktopstate.ProjectState{{ID: "p1", Name: "Project"}},
			Sessions: []desktopstate.SessionState{
				{ID: "s1", ProjectID: "p1", AgentID: "protonman", Title: ""},
				{ID: "s2", ProjectID: "p1", AgentID: "protonman", Title: "   "},
			},
		},
	}
	cache := BuildRows(model)
	rows := cache.Rows()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	for i := 1; i <= 2; i++ {
		if rows[i].Title != "Untitled conversation" {
			t.Fatalf("row %d title = %q, want 'Untitled conversation'", i, rows[i].Title)
		}
	}
}

func TestEnsureVisibleAdjustsListPosition(t *testing.T) {
	component := New()
	rows := make([]Row, 20)
	for i := range rows {
		rows[i] = Row{Kind: SessionRow, SessionID: fmt.Sprintf("s%d", i)}
	}

	// Active session at index 15 should move scroll position down
	component.EnsureVisible("s15", rows)
	if component.ScrollPosition().First <= 0 {
		t.Fatalf("expected scroll position to advance for s15, got %d", component.ScrollPosition().First)
	}

	// Active session at index 2 should scroll back up
	component.EnsureVisible("s2", rows)
	if component.ScrollPosition().First > 2 {
		t.Fatalf("expected scroll position to scroll up for s2, got %d", component.ScrollPosition().First)
	}
}

func TestMatchesQueryMatchesSessionID(t *testing.T) {
	row := Row{Kind: SessionRow, SessionID: "session-12345-abc", Title: "General Chat", Subtitle: "Protonman", ProjectID: "p1"}
	if !matchesQuery(row, "12345") {
		t.Fatal("expected matchesQuery to match SessionID substring")
	}
	if !matchesQuery(row, "session-12345") {
		t.Fatal("expected matchesQuery to match SessionID prefix")
	}
	if matchesQuery(row, "nonexistent") {
		t.Fatal("expected matchesQuery to not match nonexistent query")
	}
}

func TestSyncSessionButtonsManagesProjectNewButtons(t *testing.T) {
	c := New()
	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{
			{ID: "p1", Name: "Project 1"},
			{ID: "p2", Name: "Project 2"},
		},
	}
	c.SyncSessionButtons(state, 1)
	if !c.HasProjectButton("p1") || !c.HasProjectNewButton("p1") {
		t.Fatal("expected p1 project and new buttons to exist")
	}
	if !c.HasProjectButton("p2") || !c.HasProjectNewButton("p2") {
		t.Fatal("expected p2 project and new buttons to exist")
	}

	// Update state removing p2
	state2 := desktopstate.State{
		Projects: []desktopstate.ProjectState{
			{ID: "p1", Name: "Project 1"},
		},
	}
	c.SyncSessionButtons(state2, 2)
	if !c.HasProjectNewButton("p1") {
		t.Fatal("expected p1 project new button to be retained")
	}
	if c.HasProjectNewButton("p2") {
		t.Fatal("expected p2 project new button to be pruned")
	}
}
