//go:build desktop || desktop_gio

package controller

import (
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestDefaultProfileIsProtonman(t *testing.T) {
	profile := DefaultACPAgentProfile()
	if profile.ID != ProtonmanAgentID {
		t.Fatalf("profile ID = %q, want %q", profile.ID, ProtonmanAgentID)
	}
	if len(profile.Args) != 1 || profile.Args[0] != "--acp" {
		t.Fatalf("profile args = %#v, want [--acp]", profile.Args)
	}
}

func TestMarkDisconnectedResetsProtonConnection(t *testing.T) {
	c := newTestController()
	protonClient := c.client
	c.state.Sessions = []desktopstate.SessionState{
		{ID: "proton-session", AgentID: ProtonmanAgentID, Status: desktopstate.TaskRunning},
	}

	c.markDisconnected(protonClient)

	if c.client != nil {
		t.Fatal("client was not cleared")
	}
	if c.connection != ConnectionReconnecting {
		t.Fatalf("connection = %q, want reconnecting", c.connection)
	}
	if c.state.Sessions[0].Status != desktopstate.TaskPaused {
		t.Fatalf("session status = %q", c.state.Sessions[0].Status)
	}
}
