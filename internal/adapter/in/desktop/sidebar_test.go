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
