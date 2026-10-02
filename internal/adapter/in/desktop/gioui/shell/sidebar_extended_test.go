//go:build desktop || desktop_gio

package shell

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
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

func TestBuildSidebarRowsWithOptions_PinnedAndCustomTitles(t *testing.T) {
	activity := time.Date(2026, 9, 27, 8, 0, 0, 0, time.UTC)
	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{
			{ID: "p1", Name: "Project Alpha"},
			{ID: "p2", Name: "Project Beta"},
		},
		Sessions: []desktopstate.SessionState{
			{ID: "s1", ProjectID: "p1", AgentID: controller.ProtonmanAgentID, Title: "Default S1", Status: desktopstate.TaskIdle, LastActivityAt: activity},
			{ID: "s2", ProjectID: "p1", AgentID: controller.ProtonmanAgentID, Title: "Pinned S2", Status: desktopstate.TaskRunning, LastActivityAt: activity.Add(time.Minute)},
			{ID: "s3", ProjectID: "p2", AgentID: controller.ProtonmanAgentID, Title: "Default S3", Status: desktopstate.TaskCompleted, LastActivityAt: activity.Add(2 * time.Minute)},
		},
	}
	profiles := []app.ACPAgentProfile{controller.DefaultACPAgentProfile()}
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
		Sessions: []desktopstate.SessionState{{ID: "s1", ProjectID: "p1", AgentID: controller.ProtonmanAgentID, Title: "T"}},
	}
	profiles := []app.ACPAgentProfile{{ID: controller.ProtonmanAgentID, DisplayName: "Protonman"}}
	_, cache := buildSidebarRowsWithOptions(state, profiles, nil, nil)

	if !cache.matchesWithOptions(state, profiles, cache.filterMode, []string{}, nil) {
		t.Fatal("nil and empty pinned sets should compare equal")
	}
	if cache.matchesWithOptions(state, profiles, cache.filterMode, []string{"s1"}, nil) {
		t.Fatal("cache matched after the pinned set changed")
	}
}
