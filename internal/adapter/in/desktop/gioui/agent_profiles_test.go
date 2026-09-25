//go:build desktop || desktop_gio

package gioui

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

type acpAgentSaveRepository struct {
	saved chan []app.ACPAgentProfile
}

func (r *acpAgentSaveRepository) Load(context.Context) ([]app.ACPAgentProfile, error) {
	return nil, nil
}

func (r *acpAgentSaveRepository) Save(_ context.Context, profiles []app.ACPAgentProfile) error {
	cloned := make([]app.ACPAgentProfile, len(profiles))
	for index, profile := range profiles {
		cloned[index] = cloneACPAgentProfile(profile)
	}
	r.saved <- cloned
	return nil
}

func TestCommandSpecForACPAgentResolvesEnvironmentAtRuntime(t *testing.T) {
	t.Setenv("CUSTOM_ACP_TOKEN", "secret")
	spec := commandSpecForACPAgent(app.ACPAgentProfile{
		ID:      "custom",
		Command: "custom-acp",
		Args:    []string{"--stdio"},
		Env:     []string{"CUSTOM_ACP_TOKEN"},
	})
	if spec.Path != "custom-acp" || !slices.Equal(spec.Args, []string{"--stdio"}) {
		t.Fatalf("command = %#v", spec)
	}
	if !slices.Equal(spec.Env, []string{"CUSTOM_ACP_TOKEN=secret"}) {
		t.Fatalf("environment = %#v", spec.Env)
	}
}

func TestCommandSpecForACPAgentRetainsOverrideEnvironmentValues(t *testing.T) {
	spec := commandSpecForACPAgent(app.ACPAgentProfile{
		ID:      "custom",
		Command: "custom-acp",
		Env:     []string{"CUSTOM_ACP_TOKEN=override-secret"},
	})
	if !slices.Equal(spec.Env, []string{"CUSTOM_ACP_TOKEN=override-secret"}) {
		t.Fatalf("environment = %#v", spec.Env)
	}
}

func TestACPAgentProfileFromEditorValidatesAndScrubsEnvironment(t *testing.T) {
	profile, err := acpaentProfileFromEditor(" Reviewer ", " Review Agent ", " reviewer-acp ", `["--stdio"]`, `["TOKEN=secret","TOKEN"]`)
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "reviewer" || profile.DisplayName != "Review Agent" || profile.Command != "reviewer-acp" {
		t.Fatalf("profile = %#v", profile)
	}
	if !slices.Equal(profile.Args, []string{"--stdio"}) || !slices.Equal(profile.Env, []string{"TOKEN"}) {
		t.Fatalf("profile lists = %#v", profile)
	}
}

func TestSaveAgentProfileRejectsDuplicateID(t *testing.T) {
	repository := &acpAgentSaveRepository{saved: make(chan []app.ACPAgentProfile, 1)}
	controller := newTestController()
	controller.agentProfiles = app.NewACPAgents(repository)
	controller.profiles["reviewer"] = app.ACPAgentProfile{ID: "reviewer", DisplayName: "Reviewer", Command: "reviewer"}

	controller.saveAgentProfile(controllerAgentID, "reviewer", "Replacement", "replacement", `[]`, `[]`)

	select {
	case saved := <-repository.saved:
		t.Fatalf("duplicate profile was persisted: %#v", saved)
	case <-time.After(100 * time.Millisecond):
	}
	if status := controller.statuses[controllerAgentID]; !strings.Contains(status, "already exists") {
		t.Fatalf("status = %q, want duplicate-ID error", status)
	}
}

func TestSelectAgentUpdatesActiveProjectDefault(t *testing.T) {
	controller := newTestController()
	controller.state.Projects = []desktopstate.ProjectState{{ID: "project", AgentIDs: []string{controllerAgentID}}}
	controller.state.ActiveProjectID = "project"
	controller.profiles["reviewer"] = app.ACPAgentProfile{ID: "reviewer", DisplayName: "Reviewer", Command: "reviewer"}
	controller.connections["reviewer"] = connectionConnected
	controller.statuses["reviewer"] = "Connected"

	controller.selectAgent("reviewer")

	if controller.activeAgentID != "reviewer" {
		t.Fatalf("active agent = %q", controller.activeAgentID)
	}
	if controller.state.Projects[0].DefaultAgentID != "reviewer" || !slices.Contains(controller.state.Projects[0].AgentIDs, "reviewer") {
		t.Fatalf("project defaults = %#v", controller.state.Projects[0])
	}
}

func TestSessionConnectionUsesOwningAgent(t *testing.T) {
	snapshot := controllerSnapshot{
		ActiveAgentID:    "reviewer",
		Connection:       connectionConnected,
		AgentConnections: map[string]connectionPhase{controllerAgentID: connectionReconnecting, "reviewer": connectionConnected},
	}
	if got := sessionConnection(snapshot, controllerAgentID); got != connectionReconnecting {
		t.Fatalf("proton connection = %q", got)
	}
	if got := sessionConnection(snapshot, "reviewer"); got != connectionConnected {
		t.Fatalf("reviewer connection = %q", got)
	}
	if !anyAgentConnected(snapshot) {
		t.Fatal("a connected ACP agent was not detected")
	}
}

func TestProjectSessionsPreservesOtherAgentSessions(t *testing.T) {
	state := desktopstate.State{
		ActiveSessionID: "proton-session",
		ActiveProjectID: "project",
		Projects: []desktopstate.ProjectState{{
			ID: "project", Name: "Project", DefaultAgentID: controllerAgentID, AgentIDs: []string{controllerAgentID},
		}},
		Sessions: []desktopstate.SessionState{
			{ID: "proton-session", ProjectID: "project", AgentID: controllerAgentID, Title: "Proton"},
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
	if next.Sessions[0].ID != "proton-session" || next.Sessions[0].AgentID != controllerAgentID {
		t.Fatalf("proton session was replaced: %#v", next.Sessions[0])
	}
	if next.Sessions[1].AgentID != "reviewer" || next.Sessions[1].Title != "Review updated" || next.Sessions[2].AgentID != "reviewer" {
		t.Fatalf("reviewer projection = %#v", next.Sessions[1:])
	}
	if len(next.Projects) != 2 || next.Projects[0].ID != "project" || next.Projects[0].DefaultAgentID != controllerAgentID {
		t.Fatalf("projects = %#v", next.Projects)
	}
}

func TestMarkAgentDisconnectedLeavesOtherAgentStateIntact(t *testing.T) {
	controller := newTestController()
	protonClient := controller.clients[controllerAgentID]
	reviewerClient := &acpclient.Client{}
	controller.clients["reviewer"] = reviewerClient
	controller.connections["reviewer"] = connectionConnected
	controller.state.Sessions = []desktopstate.SessionState{
		{ID: "proton-session", AgentID: controllerAgentID, Status: desktopstate.TaskRunning},
		{ID: "review-session", AgentID: "reviewer", Status: desktopstate.TaskWaitingPermission},
	}
	controller.state.PermissionInbox = []desktopstate.PermissionRequest{
		{RequestID: "proton-permission", SessionID: "proton-session"},
		{RequestID: "review-permission", SessionID: "review-session"},
	}
	controller.permissionWait["proton-permission"] = make(chan string, 1)
	controller.permissionWait["review-permission"] = make(chan string, 1)
	controller.histories["review-session"] = historyStateLoading
	controller.historyStaging["review-session"] = []desktopstate.Event{{Kind: desktopstate.EventPromptStarted, SessionID: "review-session"}}

	controller.markAgentDisconnected("reviewer", reviewerClient)

	if controller.clients[controllerAgentID] != protonClient || controller.state.Sessions[0].Status != desktopstate.TaskRunning {
		t.Fatalf("proton state changed: %#v", controller.state)
	}
	if controller.state.Sessions[1].Status != desktopstate.TaskPaused {
		t.Fatalf("reviewer status = %q", controller.state.Sessions[1].Status)
	}
	if len(controller.state.PermissionInbox) != 1 || controller.state.PermissionInbox[0].RequestID != "proton-permission" {
		t.Fatalf("permissions = %#v", controller.state.PermissionInbox)
	}
	if controller.permissionWait["proton-permission"] == nil || controller.permissionWait["review-permission"] != nil {
		t.Fatalf("permission waiters = %#v", controller.permissionWait)
	}
	if controller.histories["review-session"] != historyStateUnloaded || len(controller.historyStaging["review-session"]) != 0 {
		t.Fatalf("reviewer history = %#v", controller.histories)
	}
}

func TestSaveAgentProfilePersistsAndRequiresRestart(t *testing.T) {
	repository := &acpAgentSaveRepository{saved: make(chan []app.ACPAgentProfile, 1)}
	controller := newTestController()
	controller.agentProfiles = app.NewACPAgents(repository)

	controller.saveAgentProfile("", "reviewer", "Reviewer", "reviewer-acp", `["--stdio"]`, `["TOKEN"]`)

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
		controller.mu.RLock()
		mutating := controller.agentMutation
		controller.mu.RUnlock()
		if !mutating {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent mutation did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	controller.mu.RLock()
	_, exists := controller.profiles["reviewer"]
	status := controller.statuses[controllerAgentID]
	controller.mu.RUnlock()
	if !exists {
		t.Fatalf("profiles = %#v", controller.profiles)
	}
	if status != "ACP agent saved · restart Desktop to apply" {
		t.Fatalf("status = %q", status)
	}
}
