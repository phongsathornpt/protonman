//go:build desktop || desktop_gio

package controller

import (
	"errors"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestProjectSessionsPreservesOtherAgentSessions(t *testing.T) {
	state := desktopstate.State{
		ActiveSessionID: "proton-session",
		ActiveProjectID: "project",
		Projects: []desktopstate.ProjectState{{
			ID: "project", Name: "Project", DefaultAgentID: ProtonmanAgentID, AgentIDs: []string{ProtonmanAgentID},
		}},
		Sessions: []desktopstate.SessionState{
			{ID: "proton-session", ProjectID: "project", AgentID: ProtonmanAgentID, Title: "Proton"},
			{ID: "review-session", ProjectID: "project", AgentID: "reviewer", Title: "Review"},
		},
	}
	next := projectSessions(state, "reviewer", []acpSession{
		{ID: "review-session", Title: "Review updated"},
		{ID: "review-new", Title: "New review", WorkspaceKey: "project"},
	})
	if len(next.Sessions) != 3 {
		t.Fatalf("sessions = %#v", next.Sessions)
	}
	if next.Sessions[0].ID != "proton-session" || next.Sessions[0].AgentID != ProtonmanAgentID {
		t.Fatalf("proton session was replaced: %#v", next.Sessions[0])
	}
	if next.Sessions[1].AgentID != "reviewer" || next.Sessions[1].Title != "Review updated" || next.Sessions[2].AgentID != "reviewer" {
		t.Fatalf("reviewer projection = %#v", next.Sessions[1:])
	}
	if len(next.Projects) != 2 || next.Projects[0].ID != "project" || next.Projects[0].DefaultAgentID != ProtonmanAgentID {
		t.Fatalf("projects = %#v", next.Projects)
	}
}

func TestScanDeviceACPAgents(t *testing.T) {
	mockLookPath := func(cmd string) (string, error) {
		switch cmd {
		case "protonman":
			return "/usr/local/bin/protonman", nil
		case "cline":
			return "/home/user/.nvm/bin/cline", nil
		default:
			return "", errors.New("not found")
		}
	}

	detected := scanDeviceACPAgents(mockLookPath)
	if len(detected) != 2 {
		t.Fatalf("detected %d agents, want 2", len(detected))
	}

	var foundProtonman, foundCline bool
	for _, a := range detected {
		if a.ID == "protonman" {
			foundProtonman = true
		}
		if a.ID == "cline" {
			foundCline = true
			if a.Command != "cline" {
				t.Fatalf("cline command = %q, want cline", a.Command)
			}
			if len(a.Args) != 1 || a.Args[0] != "--acp" {
				t.Fatalf("cline args = %v, want [--acp]", a.Args)
			}
		}
	}
	if !foundProtonman || !foundCline {
		t.Fatalf("expected protonman and cline, got %+v", detected)
	}
}
