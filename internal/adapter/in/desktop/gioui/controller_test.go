//go:build desktop || desktop_gio

package gioui

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestProjectSessionsBuildsLiveWorkspaceIndex(t *testing.T) {
	current := desktopstate.State{
		ActiveSessionID: "session-one",
		ActiveProjectID: "old-project",
		Sessions: []desktopstate.SessionState{
			{
				ID:       "session-one",
				Title:    "Existing title",
				Status:   desktopstate.TaskRunning,
				Timeline: []desktopstate.TimelineItem{{ID: "tool-1", Kind: desktopstate.TimelineTool, Title: "read"}},
			},
		},
	}
	next := projectSessions(current, controllerAgentID, []acpSession{
		{ID: "session-one", Cwd: "/workspace/alpha"},
		{ID: "session-two", Cwd: "/workspace/beta", Title: "Beta work"},
	})

	if next.ActiveSessionID != "session-one" {
		t.Fatalf("active session = %q, want session-one", next.ActiveSessionID)
	}
	if next.ActiveProjectID != "workspace:/workspace/alpha" {
		t.Fatalf("active project = %q", next.ActiveProjectID)
	}
	if len(next.Projects) != 2 {
		t.Fatalf("project count = %d, want 2", len(next.Projects))
	}
	if next.Projects[0].Name != "alpha" || next.Projects[1].Name != "beta" {
		t.Fatalf("project order = %q, %q", next.Projects[0].Name, next.Projects[1].Name)
	}
	first := next.Sessions[0]
	if first.Title != "Existing title" || first.Status != desktopstate.TaskRunning {
		t.Fatalf("existing session state was not preserved: %+v", first)
	}
	if len(first.Timeline) != 1 || first.Timeline[0].ID != "tool-1" {
		t.Fatalf("existing timeline was not preserved: %+v", first.Timeline)
	}
	if first.AgentID != controllerAgentID {
		t.Fatalf("agent ID = %q, want %q", first.AgentID, controllerAgentID)
	}
}

func TestProjectSessionsReplacesMissingSelection(t *testing.T) {
	current := desktopstate.State{
		ActiveSessionID: "removed",
		Sessions:        []desktopstate.SessionState{{ID: "removed"}},
	}
	next := projectSessions(current, controllerAgentID, []acpSession{{ID: "remaining", Cwd: "/workspace/remaining"}})
	if next.ActiveSessionID != "remaining" {
		t.Fatalf("active session = %q, want remaining", next.ActiveSessionID)
	}
	if next.ActiveProjectID != "workspace:/workspace/remaining" {
		t.Fatalf("active project = %q", next.ActiveProjectID)
	}
}

func TestDeriveProjectsKeepsOnePrimaryFolder(t *testing.T) {
	projects := deriveProjects([]desktopstate.SessionState{
		{ID: "one", ProjectID: "project", Workspace: "/workspace", AgentID: controllerAgentID},
		{ID: "two", ProjectID: "project", Workspace: "/workspace", AgentID: controllerAgentID},
	})
	if len(projects) != 1 || len(projects[0].Folders) != 1 {
		t.Fatalf("projects = %+v", projects)
	}
	if !projects[0].Folders[0].Primary {
		t.Fatal("project folder must be primary")
	}
}

func TestNewSessionWorkspaceUsesAuthoritativeOverride(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("PROTONMAN_GIO_WORKSPACE", workspace)
	got, err := newSessionWorkspace(desktopstate.State{})
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("workspace = %q, want %q", got, want)
	}
}

func TestNewSessionWorkspaceRejectsMissingOverride(t *testing.T) {
	t.Setenv("PROTONMAN_GIO_WORKSPACE", filepath.Join(t.TempDir(), "missing"))
	if _, err := newSessionWorkspace(desktopstate.State{}); err == nil {
		t.Fatal("expected missing workspace override to fail")
	}
}

func TestResolveACPBinaryFor(t *testing.T) {
	if got := resolveACPBinaryFor("  /custom/protonman  ", "/app/protonman-desktop-gio", func(string) bool { return true }); got != "/custom/protonman" {
		t.Fatalf("override binary = %q", got)
	}
	candidate := filepath.Join(string(filepath.Separator), "app", "libexec", "protonman")
	got := resolveACPBinaryFor("", filepath.Join(string(filepath.Separator), "app", "protonman-desktop-gio"), func(path string) bool {
		return path == candidate
	})
	if got != candidate {
		t.Fatalf("bundled binary = %q, want %q", got, candidate)
	}
	if got := resolveACPBinaryFor("", "", nil); got != "protonman" {
		t.Fatalf("fallback binary = %q", got)
	}
}

func TestCompactErrorPreservesUTF8(t *testing.T) {
	got := compactError(errorString(strings.Repeat("ก", 130)))
	if !utf8.ValidString(got) {
		t.Fatalf("compacted error is not UTF-8: %q", got)
	}
	if utf8.RuneCountInString(got) != 118 {
		t.Fatalf("compacted rune count = %d, want 118", utf8.RuneCountInString(got))
	}
}

func TestShortIDPreservesUTF8(t *testing.T) {
	got := shortID(strings.Repeat("ก", 10))
	if got != strings.Repeat("ก", 8) {
		t.Fatalf("short ID = %q", got)
	}
}

func TestSnapshotCachesUntilRevisionChanges(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "session-1"
	controller.state.Sessions[0].Timeline = []desktopstate.TimelineItem{{Kind: desktopstate.TimelineTool, ID: "item", Text: "before"}}
	controller.revision = 1

	first := controller.snapshot()
	second := controller.snapshot()
	if &first.State.Sessions[0] != &second.State.Sessions[0] {
		t.Fatal("unchanged controller revision rebuilt the snapshot")
	}

	controller.mu.Lock()
	desktopstate.Apply(&controller.state, desktopstate.Event{
		Kind:      desktopstate.EventTimelineUpserted,
		SessionID: controller.state.ActiveSessionID,
		Item: desktopstate.TimelineItem{
			Kind: desktopstate.TimelineTool,
			ID:   "item",
			Text: "after",
		},
	})
	controller.revision++
	controller.mu.Unlock()

	third := controller.snapshot()
	fourth := controller.snapshot()
	if first.State.Sessions[0].Timeline[0].Text != "before" {
		t.Fatalf("cached snapshot changed with controller state: %#v", first.State.Sessions[0].Timeline)
	}
	if third.State.Sessions[0].Timeline[0].Text != "after" {
		t.Fatalf("refreshed snapshot = %#v", third.State.Sessions[0].Timeline)
	}
	if &third.State.Sessions[0] != &fourth.State.Sessions[0] {
		t.Fatal("refreshed controller revision rebuilt the snapshot twice")
	}
}

type errorString string

func (err errorString) Error() string {
	return string(err)
}
