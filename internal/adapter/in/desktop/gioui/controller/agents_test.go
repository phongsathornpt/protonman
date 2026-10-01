//go:build desktop || desktop_gio

package controller

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// acpAgentSaveRepository records persisted agent profiles so tests can assert on
// exactly what the coordinator wrote.
type acpAgentSaveRepository struct {
	saved chan []app.ACPAgentProfile
}

func (r *acpAgentSaveRepository) Load(context.Context) ([]app.ACPAgentProfile, error) {
	return nil, nil
}

func (r *acpAgentSaveRepository) Save(_ context.Context, profiles []app.ACPAgentProfile) error {
	cloned := make([]app.ACPAgentProfile, len(profiles))
	for index, profile := range profiles {
		cloned[index] = CloneACPAgentProfile(profile)
	}
	r.saved <- cloned
	return nil
}

func TestSaveAgentProfileRejectsDuplicateID(t *testing.T) {
	repository := &acpAgentSaveRepository{saved: make(chan []app.ACPAgentProfile, 1)}
	c := newTestController()
	c.agentProfiles = app.NewACPAgents(repository)
	c.profiles["reviewer"] = app.ACPAgentProfile{ID: "reviewer", DisplayName: "Reviewer", Command: "reviewer"}

	c.SaveAgentProfile(ProtonmanAgentID, "reviewer", "Replacement", "replacement", `[]`, `[]`)

	select {
	case saved := <-repository.saved:
		t.Fatalf("duplicate profile was persisted: %#v", saved)
	case <-time.After(100 * time.Millisecond):
	}
	if status := c.statuses[ProtonmanAgentID]; !strings.Contains(status, "already exists") {
		t.Fatalf("status = %q, want duplicate-ID error", status)
	}
}

func TestSelectAgentUpdatesActiveProjectDefault(t *testing.T) {
	c := newTestController()
	c.state.Projects = []desktopstate.ProjectState{{ID: "project", AgentIDs: []string{ProtonmanAgentID}}}
	c.state.ActiveProjectID = "project"
	c.profiles["reviewer"] = app.ACPAgentProfile{ID: "reviewer", DisplayName: "Reviewer", Command: "reviewer"}
	c.connections["reviewer"] = ConnectionConnected
	c.statuses["reviewer"] = "Connected"

	c.SelectAgent("reviewer")

	if c.activeAgentID != "reviewer" {
		t.Fatalf("active agent = %q", c.activeAgentID)
	}
	if c.state.Projects[0].DefaultAgentID != "reviewer" || !slices.Contains(c.state.Projects[0].AgentIDs, "reviewer") {
		t.Fatalf("project defaults = %#v", c.state.Projects[0])
	}
}

func TestSelectAgentSwitchesToMatchingAgentSession(t *testing.T) {
	c := newTestController()
	c.state.Projects = []desktopstate.ProjectState{{ID: "project", AgentIDs: []string{ProtonmanAgentID, "cline"}}}
	c.state.ActiveProjectID = "project"
	c.state.Sessions = []desktopstate.SessionState{
		{ID: "sess-proton", ProjectID: "project", AgentID: ProtonmanAgentID, AvailableModels: []string{"claude-3-7-sonnet"}},
		{ID: "sess-cline", ProjectID: "project", AgentID: "cline", AvailableModels: []string{"claude-3-5-sonnet", "deepseek-coder"}},
	}
	c.state.ActiveSessionID = "sess-proton"
	c.activeAgentID = ProtonmanAgentID
	c.profiles["cline"] = app.ACPAgentProfile{ID: "cline", DisplayName: "Cline", Command: "cline"}
	c.connections["cline"] = ConnectionConnected
	c.statuses["cline"] = "Connected"

	c.SelectAgent("cline")

	if c.activeAgentID != "cline" {
		t.Fatalf("active agent = %q, want cline", c.activeAgentID)
	}
	if c.state.ActiveSessionID != "sess-cline" {
		t.Fatalf("active session ID = %q, want sess-cline", c.state.ActiveSessionID)
	}
	activeSession, ok := SessionByID(c.state, c.state.ActiveSessionID)
	if !ok {
		t.Fatal("expected active session found")
	}
	if len(activeSession.AvailableModels) != 2 || activeSession.AvailableModels[0] != "claude-3-5-sonnet" {
		t.Fatalf("expected cline available models, got %#v", activeSession.AvailableModels)
	}
}

func TestMarkAgentDisconnectedLeavesOtherAgentStateIntact(t *testing.T) {
	c := newTestController()
	protonClient := c.clients[ProtonmanAgentID]
	reviewerClient := &acpclient.Client{}
	c.clients["reviewer"] = reviewerClient
	c.connections["reviewer"] = ConnectionConnected
	c.state.Sessions = []desktopstate.SessionState{
		{ID: "proton-session", AgentID: ProtonmanAgentID, Status: desktopstate.TaskRunning},
		{ID: "review-session", AgentID: "reviewer", Status: desktopstate.TaskWaitingPermission},
	}
	c.state.PermissionInbox = []desktopstate.PermissionRequest{
		{RequestID: "proton-permission", SessionID: "proton-session"},
		{RequestID: "review-permission", SessionID: "review-session"},
	}
	c.permissionWait["proton-permission"] = make(chan string, 1)
	c.permissionWait["review-permission"] = make(chan string, 1)
	c.histories["review-session"] = HistoryStateLoading
	c.historyStaging["review-session"] = []desktopstate.Event{{Kind: desktopstate.EventPromptStarted, SessionID: "review-session"}}

	c.markAgentDisconnected("reviewer", reviewerClient)

	if c.clients[ProtonmanAgentID] != protonClient || c.state.Sessions[0].Status != desktopstate.TaskRunning {
		t.Fatalf("proton state changed: %#v", c.state)
	}
	if c.state.Sessions[1].Status != desktopstate.TaskPaused {
		t.Fatalf("reviewer status = %q", c.state.Sessions[1].Status)
	}
	if len(c.state.PermissionInbox) != 1 || c.state.PermissionInbox[0].RequestID != "proton-permission" {
		t.Fatalf("permissions = %#v", c.state.PermissionInbox)
	}
	if c.permissionWait["proton-permission"] == nil || c.permissionWait["review-permission"] != nil {
		t.Fatalf("permission waiters = %#v", c.permissionWait)
	}
	if c.histories["review-session"] != HistoryStateUnloaded || len(c.historyStaging["review-session"]) != 0 {
		t.Fatalf("reviewer history = %#v", c.histories)
	}
}

func TestSaveAgentProfilePersistsAndRequiresRestart(t *testing.T) {
	repository := &acpAgentSaveRepository{saved: make(chan []app.ACPAgentProfile, 1)}
	c := newTestController()
	c.agentProfiles = app.NewACPAgents(repository)

	c.SaveAgentProfile("", "reviewer", "Reviewer", "reviewer-acp", `["--stdio"]`, `["TOKEN"]`)

	select {
	case saved := <-repository.saved:
		if len(saved) != 2 || saved[1].ID != "reviewer" {
			t.Fatalf("saved profiles = %#v", saved)
		}
	case <-time.After(time.Second):
		t.Fatal("agent profile save did not complete")
	}
	deadline := time.Now().Add(time.Second)
	for {
		c.mu.RLock()
		mutating := c.agentMutation
		c.mu.RUnlock()
		if !mutating {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent mutation did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	c.mu.RLock()
	_, exists := c.profiles["reviewer"]
	status := c.statuses[ProtonmanAgentID]
	c.mu.RUnlock()
	if !exists {
		t.Fatalf("profiles = %#v", c.profiles)
	}
	if status != "ACP agent saved · restart Desktop to apply" {
		t.Fatalf("status = %q", status)
	}
}

func TestPruneAgentRuntimeDropsUnusedRemovedProfiles(t *testing.T) {
	c := newTestController()
	c.profiles["removed"] = app.ACPAgentProfile{ID: "removed", Command: "removed-acp"}
	c.profiles["live-session"] = app.ACPAgentProfile{ID: "live-session", Command: "session-acp"}
	c.profiles["live-client"] = app.ACPAgentProfile{ID: "live-client", Command: "client-acp"}
	c.connections["removed"] = ConnectionReconnecting
	c.statuses["removed"] = "Restart required to connect"
	c.connections["live-session"] = ConnectionConnected
	c.statuses["live-session"] = "Connected"
	c.clients["live-client"] = &acpclient.Client{}
	c.connections["live-client"] = ConnectionConnected
	c.statuses["live-client"] = "Connected"
	c.state.Sessions = append(c.state.Sessions, desktopstate.SessionState{
		ID:      "existing-session",
		AgentID: "live-session",
	})

	c.mu.Lock()
	c.profiles = map[string]app.ACPAgentProfile{
		ProtonmanAgentID: DefaultACPAgentProfile(),
	}
	c.pruneAgentRuntimeLocked()
	c.mu.Unlock()

	if _, ok := c.connections["removed"]; ok {
		t.Fatal("removed profile connection state was retained")
	}
	if _, ok := c.statuses["removed"]; ok {
		t.Fatal("removed profile status was retained")
	}
	for _, agentID := range []string{"live-session", "live-client", ProtonmanAgentID} {
		if _, ok := c.connections[agentID]; !ok {
			t.Fatalf("live connection state for %q was pruned", agentID)
		}
		if _, ok := c.statuses[agentID]; !ok {
			t.Fatalf("live status for %q was pruned", agentID)
		}
	}
}
