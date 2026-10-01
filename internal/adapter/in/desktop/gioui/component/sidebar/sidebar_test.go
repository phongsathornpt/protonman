//go:build desktop || desktop_gio

package sidebar

import (
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
