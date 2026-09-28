//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

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

func TestAgentSessionLockRegistryReleasesUnusedKeys(t *testing.T) {
	var registry agentSessionLockRegistry
	for index := range 1024 {
		unlock := registry.lock(fmt.Sprintf("agent-%d", index))
		unlock()
	}
	if count := len(registry.locks); count != 0 {
		t.Fatalf("retained %d idle agent locks", count)
	}
}

func TestAgentSessionLockRegistryKeepsWaitersOnSameLock(t *testing.T) {
	var registry agentSessionLockRegistry
	firstUnlock := registry.lock("reviewer")
	acquired := make(chan func(), 1)
	finished := make(chan struct{})
	go func() {
		unlock := registry.lock("reviewer")
		acquired <- unlock
		close(finished)
	}()

	select {
	case <-acquired:
		t.Fatal("second operation acquired the agent lock concurrently")
	case <-time.After(10 * time.Millisecond):
	}
	firstUnlock()

	var secondUnlock func()
	select {
	case secondUnlock = <-acquired:
	case <-time.After(time.Second):
		t.Fatal("waiting operation did not acquire the agent lock")
	}
	if count := len(registry.locks); count != 1 {
		t.Fatalf("lock entries while held = %d, want 1", count)
	}
	secondUnlock()
	<-finished
	if count := len(registry.locks); count != 0 {
		t.Fatalf("retained %d released agent locks", count)
	}
}

func TestAgentSessionLockRegistryCancelsWaitingAcquisition(t *testing.T) {
	var registry agentSessionLockRegistry
	firstUnlock := registry.lock("reviewer")
	ctx, cancel := context.WithCancel(context.Background())
	acquired := make(chan bool, 1)
	go func() {
		unlock, ok := registry.lockContext(ctx, "reviewer")
		if ok {
			unlock()
		}
		acquired <- ok
	}()
	cancel()

	select {
	case ok := <-acquired:
		if ok {
			t.Fatal("cancelled waiter acquired the agent lock")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled waiter remained blocked")
	}
	if count := registry.locks["reviewer"].refs; count != 1 {
		t.Fatalf("retained waiter references = %d, want only the holder", count)
	}
	firstUnlock()
	if count := len(registry.locks); count != 0 {
		t.Fatalf("retained %d released agent locks", count)
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

func TestPruneAgentRuntimeDropsUnusedRemovedProfiles(t *testing.T) {
	controller := newTestController()
	controller.profiles["removed"] = app.ACPAgentProfile{ID: "removed", Command: "removed-acp"}
	controller.profiles["live-session"] = app.ACPAgentProfile{ID: "live-session", Command: "session-acp"}
	controller.profiles["live-client"] = app.ACPAgentProfile{ID: "live-client", Command: "client-acp"}
	controller.connections["removed"] = connectionReconnecting
	controller.statuses["removed"] = "Restart required to connect"
	controller.connections["live-session"] = connectionConnected
	controller.statuses["live-session"] = "Connected"
	controller.clients["live-client"] = &acpclient.Client{}
	controller.connections["live-client"] = connectionConnected
	controller.statuses["live-client"] = "Connected"
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{
		ID:      "existing-session",
		AgentID: "live-session",
	})

	controller.mu.Lock()
	controller.profiles = map[string]app.ACPAgentProfile{
		controllerAgentID: defaultACPAgentProfile(),
	}
	controller.pruneAgentRuntimeLocked()
	controller.mu.Unlock()

	if _, ok := controller.connections["removed"]; ok {
		t.Fatal("removed profile connection state was retained")
	}
	if _, ok := controller.statuses["removed"]; ok {
		t.Fatal("removed profile status was retained")
	}
	for _, agentID := range []string{"live-session", "live-client", controllerAgentID} {
		if _, ok := controller.connections[agentID]; !ok {
			t.Fatalf("live connection state for %q was pruned", agentID)
		}
		if _, ok := controller.statuses[agentID]; !ok {
			t.Fatalf("live status for %q was pruned", agentID)
		}
	}
}

func TestAgentProfilesPresetsAndCardLayout(t *testing.T) {
	view := newShell(newTheme("dark"))
	profiles := []app.ACPAgentProfile{
		{ID: controllerAgentID, DisplayName: "Protonman", Command: "protonman", Args: []string{"--acp"}},
		{ID: "antigravity", DisplayName: "Antigravity", Command: "agy", Args: []string{"--acp"}},
	}
	snapshot := controllerSnapshot{
		AgentProfiles:    profiles,
		ActiveAgentID:    controllerAgentID,
		AgentConnections: map[string]connectionPhase{controllerAgentID: connectionConnected, "antigravity": connectionReconnecting},
		AgentStatuses:    map[string]string{controllerAgentID: "Connected", "antigravity": "Connection failed · flags provided but not defined: -acp"},
		Connection:       connectionConnected,
		Status:           "Connected",
	}

	// 1. Layout agent profiles panel with cards
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	dims := view.layoutAgentProfilesPanel(gtx, snapshot)
	router.Frame(gtx.Ops)
	if dims.Size.X == 0 {
		t.Fatal("layoutAgentProfilesPanel returned 0 width")
	}

	// 2. Open editor for a new agent and verify presets bar
	view.agentEditorVisible = true
	view.agentEditorOriginalID = ""
	view.clearAgentProfileEditors()

	var opPreset op.Ops
	gtxPreset := layout.Context{
		Ops:         &opPreset,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtxPreset, snapshot)
	router.Frame(gtxPreset.Ops)

	// Click OpenCode preset
	view.agentPresetOpencodeBtn.Click()
	var opOpencode op.Ops
	gtxOpencode := layout.Context{
		Ops:         &opOpencode,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtxOpencode, snapshot)
	router.Frame(gtxOpencode.Ops)

	if view.agentIDEditor.Text() != "opencode" || view.agentCommandEditor.Text() != "opencode" || view.agentArgsEditor.Text() != "acp" {
		t.Fatalf("opencode preset mismatch: id=%q cmd=%q args=%q", view.agentIDEditor.Text(), view.agentCommandEditor.Text(), view.agentArgsEditor.Text())
	}

	// Click Cline preset
	view.agentPresetClineBtn.Click()
	var opCline op.Ops
	gtxCline := layout.Context{
		Ops:         &opCline,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtxCline, snapshot)
	router.Frame(gtxCline.Ops)

	if view.agentIDEditor.Text() != "cline" || view.agentCommandEditor.Text() != "cline" || view.agentArgsEditor.Text() != "--acp" {
		t.Fatalf("cline preset mismatch: id=%q cmd=%q args=%q", view.agentIDEditor.Text(), view.agentCommandEditor.Text(), view.agentArgsEditor.Text())
	}

	// Click Antigravity preset
	view.agentPresetAntigravityBtn.Click()
	var opClick op.Ops
	gtxClick := layout.Context{
		Ops:         &opClick,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtxClick, snapshot)
	router.Frame(gtxClick.Ops)

	if view.agentIDEditor.Text() != "antigravity" || view.agentCommandEditor.Text() != "agy" || view.agentArgsEditor.Text() != "--acp" {
		t.Fatalf("antigravity preset mismatch: id=%q cmd=%q args=%q", view.agentIDEditor.Text(), view.agentCommandEditor.Text(), view.agentArgsEditor.Text())
	}

	// Click Claude Code preset
	view.agentPresetClaudeBtn.Click()
	var opClaude op.Ops
	gtxClaude := layout.Context{
		Ops:         &opClaude,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtxClaude, snapshot)
	router.Frame(gtxClaude.Ops)

	if view.agentIDEditor.Text() != "claude" || view.agentCommandEditor.Text() != "claude" || view.agentArgsEditor.Text() != "--acp" {
		t.Fatalf("claude preset mismatch: id=%q cmd=%q args=%q", view.agentIDEditor.Text(), view.agentCommandEditor.Text(), view.agentArgsEditor.Text())
	}
}

func TestAgentProfileDeleteConfirmationFlow(t *testing.T) {
	view := newShell(newTheme("dark"))
	var removedID string
	view.onRemoveAgentProfile = func(id string) {
		removedID = id
	}

	profiles := []app.ACPAgentProfile{
		{ID: controllerAgentID, DisplayName: "Protonman", Command: "protonman", Args: []string{"--acp"}},
		{ID: "reviewer", DisplayName: "Reviewer", Command: "reviewer-acp", Args: []string{"--stdio"}},
	}
	snapshot := controllerSnapshot{
		AgentProfiles: profiles,
		ActiveAgentID: controllerAgentID,
	}

	// Edit reviewer
	view.agentEditorVisible = true
	view.agentEditorOriginalID = "reviewer"
	view.agentIDEditor.SetText("reviewer")
	view.agentCommandEditor.SetText("reviewer-acp")

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtx, snapshot)
	router.Frame(gtx.Ops)

	// Click Remove agent button -> Should enter confirmation mode
	view.agentRemoveButton.Click()
	var opRemove op.Ops
	gtxRemove := layout.Context{
		Ops:         &opRemove,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtxRemove, snapshot)
	router.Frame(gtxRemove.Ops)

	if !view.agentConfirmDelete {
		t.Fatal("clicking remove agent did not activate confirmation mode")
	}

	// Click Cancel in confirmation banner -> should cancel delete confirmation
	view.agentCancelDeleteBtn.Click()
	var opCancel op.Ops
	gtxCancel := layout.Context{
		Ops:         &opCancel,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtxCancel, snapshot)
	router.Frame(gtxCancel.Ops)

	if view.agentConfirmDelete {
		t.Fatal("clicking cancel did not exit delete confirmation mode")
	}
	if removedID != "" {
		t.Fatal("agent was removed despite canceling")
	}

	// Click Remove agent again and then Confirm Delete
	view.agentRemoveButton.Click()
	var opRemove2 op.Ops
	gtxRemove2 := layout.Context{
		Ops:         &opRemove2,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtxRemove2, snapshot)
	router.Frame(gtxRemove2.Ops)

	view.agentConfirmDeleteBtn.Click()
	var opConfirm op.Ops
	gtxConfirm := layout.Context{
		Ops:         &opConfirm,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtxConfirm, snapshot)
	router.Frame(gtxConfirm.Ops)

	if removedID != "reviewer" {
		t.Fatalf("onRemoveAgentProfile was not called with 'reviewer', got %q", removedID)
	}
	if view.agentEditorVisible {
		t.Fatal("editor was not closed after deletion")
	}
}

func TestAgentProfileCancelButtonClosesEditor(t *testing.T) {
	view := newShell(newTheme("dark"))
	snapshot := controllerSnapshot{
		AgentProfiles: []app.ACPAgentProfile{{ID: controllerAgentID, DisplayName: "Protonman"}},
	}
	view.agentEditorVisible = true
	view.agentIDEditor.SetText("draft")

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtx, snapshot)
	router.Frame(gtx.Ops)

	view.agentCancelButton.Click()
	var opCancel op.Ops
	gtxCancel := layout.Context{
		Ops:         &opCancel,
		Constraints: layout.Exact(image.Point{X: 600, Y: 800}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutAgentProfilesPanel(gtxCancel, snapshot)
	router.Frame(gtxCancel.Ops)

	if view.agentEditorVisible {
		t.Fatal("cancel button did not close agent editor")
	}
}

func TestAgentConfigurationCardWidthExpansion(t *testing.T) {
	view := newShell(newTheme("dark"))
	snapshot := controllerSnapshot{
		AgentProfiles: []app.ACPAgentProfile{{ID: controllerAgentID, DisplayName: "Protonman"}},
	}
	view.clearAgentProfileEditors()

	// 1. Verify layoutInspectorEditor expands to Max.X even when Min.X is 0 and editor is empty
	var operations op.Ops
	var router input.Router
	gtxEditor := layout.Context{
		Ops:         &operations,
		Constraints: layout.Constraints{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 450, Y: 400}},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	dimsEditor := view.layoutInspectorEditor(gtxEditor, "Agent ID", &view.agentIDEditor, true)
	router.Frame(gtxEditor.Ops)
	if dimsEditor.Size.X != 450 {
		t.Fatalf("layoutInspectorEditor width = %d, want 450", dimsEditor.Size.X)
	}

	// 2. Verify layoutAgentConfigurationCard expands to Max.X even when Min.X is 0
	var opCard op.Ops
	gtxCard := layout.Context{
		Ops:         &opCard,
		Constraints: layout.Constraints{Min: image.Point{X: 0, Y: 0}, Max: image.Point{X: 520, Y: 800}},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	dimsCard := view.layoutAgentConfigurationCard(gtxCard, snapshot, snapshot.AgentProfiles, true)
	router.Frame(gtxCard.Ops)
	if dimsCard.Size.X != 520 {
		t.Fatalf("layoutAgentConfigurationCard width = %d, want 520", dimsCard.Size.X)
	}
}

func TestAddACPAgentSaveFlowAndModalResponsiveness(t *testing.T) {
	view := newShell(newTheme("dark"))
	var savedOrigID, savedID, savedName, savedCmd, savedArgs, savedEnv string
	view.onSaveAgentProfile = func(origID, id, name, cmd, args, env string) {
		savedOrigID = origID
		savedID = id
		savedName = name
		savedCmd = cmd
		savedArgs = args
		savedEnv = env
	}

	snapshot := controllerSnapshot{
		AgentProfiles: []app.ACPAgentProfile{
			{ID: controllerAgentID, DisplayName: "Protonman", Command: "protonman", Args: []string{"--acp"}},
		},
	}

	// 1. Initial render with settings modal open on ACP Agents tab
	view.openSettingsModal()
	view.settingsActiveTab = 2
	if view.agentEditorVisible {
		t.Fatal("agent editor should start hidden")
	}

	var op1 op.Ops
	var router input.Router
	gtx1 := layout.Context{
		Ops:         &op1,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layout(gtx1, snapshot)
	router.Frame(gtx1.Ops)

	// 2. Click "+ Add ACP Agent" button
	view.agentFormToggleButton.Click()

	var op2 op.Ops
	gtx2 := layout.Context{
		Ops:         &op2,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(2, 0),
		Source:      router.Source(),
	}
	view.layout(gtx2, snapshot)
	router.Frame(gtx2.Ops)

	if !view.agentEditorVisible {
		t.Fatal("clicking + Add ACP Agent did not set agentEditorVisible = true")
	}
	wakeTime, shouldWake := router.WakeupTime()
	t.Logf("frame 2 wakeTime: %v, shouldWake: %v", wakeTime, shouldWake)
	for i := 0; i < 5; i++ {
		var idleOp op.Ops
		gtxIdle := layout.Context{
			Ops:         &idleOp,
			Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Unix(int64(10+i), 0),
			Source:      router.Source(),
		}
		view.layout(gtxIdle, snapshot)
		router.Frame(gtxIdle.Ops)
		wt, sw := router.WakeupTime()
		t.Logf("idle frame %d wakeTime: %v, shouldWake: %v", i, wt, sw)
	}

	// 3. Click preset "Antigravity"
	view.agentPresetAntigravityBtn.Click()

	var op3 op.Ops
	gtx3 := layout.Context{
		Ops:         &op3,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(3, 0),
		Source:      router.Source(),
	}
	view.layout(gtx3, snapshot)
	router.Frame(gtx3.Ops)

	if view.agentIDEditor.Text() != "antigravity" || view.agentCommandEditor.Text() != "agy" {
		t.Fatalf("preset not loaded into editors: id=%q cmd=%q", view.agentIDEditor.Text(), view.agentCommandEditor.Text())
	}

	// 4. Click "Save agent"
	view.agentSaveButton.Click()

	var op4 op.Ops
	gtx4 := layout.Context{
		Ops:         &op4,
		Constraints: layout.Exact(image.Point{X: 1180, Y: 760}),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(4, 0),
		Source:      router.Source(),
	}
	view.layout(gtx4, snapshot)
	router.Frame(gtx4.Ops)

	if savedID != "antigravity" || savedName != "Antigravity" || savedCmd != "agy" || savedArgs != "--acp" || savedEnv != "" || savedOrigID != "" {
		t.Fatalf("onSaveAgentProfile mismatch: origID=%q id=%q name=%q cmd=%q args=%q env=%q", savedOrigID, savedID, savedName, savedCmd, savedArgs, savedEnv)
	}
	if view.agentEditorVisible {
		t.Fatal("agent editor remained visible after successful save")
	}
	if view.agentIDEditor.Text() != "" || view.agentCommandEditor.Text() != "" {
		t.Fatalf("editors were not cleared after save: id=%q cmd=%q", view.agentIDEditor.Text(), view.agentCommandEditor.Text())
	}
}
