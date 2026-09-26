//go:build desktop || desktop_gio

package gioui

import (
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestActiveSessionScrollsIntoSidebarOnSelectionOnly(t *testing.T) {
	view := newShell(newTheme("light"))
	state := desktopstate.State{ActiveSessionID: "last", Projects: []desktopstate.ProjectState{{ID: "project", Name: "Project"}}}
	for i := 0; i < 8; i++ {
		state.Sessions = append(state.Sessions, desktopstate.SessionState{ID: string(rune('a' + i)), ProjectID: "project"})
	}
	state.Sessions = append(state.Sessions, desktopstate.SessionState{ID: "last", ProjectID: "project"})
	view.syncConversation(state)
	if view.sidebarList.Position.First != 8 {
		t.Fatalf("sidebar first = %d, want 8 near selected row", view.sidebarList.Position.First)
	}
	view.sidebarList.Position.First = 3
	view.syncConversation(state)
	if view.sidebarList.Position.First != 3 {
		t.Fatal("ordinary snapshot update changed the user's sidebar scroll")
	}
}
