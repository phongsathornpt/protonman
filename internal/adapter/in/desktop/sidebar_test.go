//go:build desktop

package desktop

import (
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestFilterSessionsMatchesTitleAndWorkspace(t *testing.T) {
	sessions := []desktopstate.SessionState{
		{ID: "one", Title: "Refactor desktop shell", WorkspaceName: "proton"},
		{ID: "two", Title: "Investigate router cache", WorkspaceName: "protonman-rs"},
	}

	byTitle := filterSessions(sessions, "DESKTOP")
	if len(byTitle) != 1 || byTitle[0].ID != "one" {
		t.Fatalf("unexpected title search result: %#v", byTitle)
	}

	byWorkspace := filterSessions(sessions, "protonman-rs")
	if len(byWorkspace) != 1 || byWorkspace[0].ID != "two" {
		t.Fatalf("unexpected workspace search result: %#v", byWorkspace)
	}
}

func TestFilterSessionsEmptyQueryPreservesAllSessions(t *testing.T) {
	sessions := []desktopstate.SessionState{{ID: "one"}, {ID: "two"}}
	got := filterSessions(sessions, "   ")
	if len(got) != len(sessions) {
		t.Fatalf("expected %d sessions, got %d", len(sessions), len(got))
	}
	if &got[0] == &sessions[0] {
		t.Fatal("expected filter to return an independent slice")
	}
}

func TestBuildSidebarRowsOrdersWorkspacesAndNewestSessionFirst(t *testing.T) {
	sessions := []desktopstate.SessionState{
		{ID: "workspace-a-20260914-090000", WorkspaceKey: "a", WorkspaceName: "zeta"},
		{ID: "workspace-b-20260914-110000", WorkspaceKey: "b", WorkspaceName: "alpha"},
		{ID: "workspace-b-20260914-100000", WorkspaceKey: "b", WorkspaceName: "alpha"},
	}

	rows := buildSidebarRows(sessions)
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}
	if rows[0].Kind != sidebarWorkspaceRow || rows[0].WorkspaceName != "alpha" {
		t.Fatalf("expected alpha workspace first, got %#v", rows[0])
	}
	if rows[1].SessionID != "workspace-b-20260914-110000" || rows[2].SessionID != "workspace-b-20260914-100000" {
		t.Fatalf("expected newest alpha session first, got %#v / %#v", rows[1], rows[2])
	}
	if rows[3].Kind != sidebarWorkspaceRow || rows[3].WorkspaceName != "zeta" {
		t.Fatalf("expected zeta workspace second, got %#v", rows[3])
	}
}

func TestBuildSidebarRowsWithCollapsedWorkspaceHidesItsSessions(t *testing.T) {
	sessions := []desktopstate.SessionState{
		{ID: "a-2", WorkspaceKey: "a", WorkspaceName: "alpha"},
		{ID: "a-1", WorkspaceKey: "a", WorkspaceName: "alpha"},
		{ID: "b-1", WorkspaceKey: "b", WorkspaceName: "beta"},
	}

	rows := buildSidebarRowsWithCollapsed(sessions, map[string]bool{"key:a": true})
	if len(rows) != 3 {
		t.Fatalf("expected two workspace rows and one expanded session, got %d", len(rows))
	}
	if rows[0].Kind != sidebarWorkspaceRow || rows[0].WorkspaceKey != "key:a" {
		t.Fatalf("expected collapsed alpha workspace row, got %#v", rows[0])
	}
	if rows[1].Kind != sidebarWorkspaceRow || rows[1].WorkspaceKey != "key:b" {
		t.Fatalf("expected beta workspace row after collapsed alpha, got %#v", rows[1])
	}
	if rows[2].Kind != sidebarSessionRow || rows[2].SessionID != "b-1" {
		t.Fatalf("expected beta session row, got %#v", rows[2])
	}
}
