//go:build desktop || desktop_gio

package controller

import (
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestProjectSessionsPreservesProtonSessions(t *testing.T) {
	state := desktopstate.State{
		ActiveSessionID: "proton-session",
		ActiveProjectID: "project",
		Projects: []desktopstate.ProjectState{{
			ID: "project", Name: "Project", DefaultAgentID: ProtonmanAgentID, AgentIDs: []string{ProtonmanAgentID},
		}},
		Sessions: []desktopstate.SessionState{
			{ID: "proton-session", ProjectID: "project", AgentID: ProtonmanAgentID, Title: "Proton"},
		},
	}
	next := projectSessions(state, ProtonmanAgentID, []acpSession{
		{ID: "proton-session", Title: "Proton updated"},
		{ID: "proton-new", Title: "New proton", WorkspaceKey: "project"},
	}, "")
	if len(next.Sessions) != 2 {
		t.Fatalf("sessions = %#v", next.Sessions)
	}
	if next.Sessions[0].AgentID != ProtonmanAgentID || next.Sessions[0].Title != "Proton updated" {
		t.Fatalf("proton projection = %#v", next.Sessions[0])
	}
}
