//go:build desktop || desktop_gio

package controller

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestInspectorResultRoutesCollidingSessionIDToOwningAgent(t *testing.T) {
	controller := newTestController()
	reviewer := &acpclient.Client{}
	controller.clients["reviewer"] = reviewer
	controller.connections["reviewer"] = ConnectionConnected
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{
		ID: "session-1", AgentID: "reviewer", Context: desktopstate.SessionContextState{Goal: "old reviewer goal"},
	})
	controller.state.ActiveSessionID = "session-1"
	controller.state.ActiveAgentID = "reviewer"

	if !controller.applySessionContext(reviewer, sessionContextResult{SessionID: "session-1", Goal: "reviewer goal"}) {
		t.Fatal("reviewer context result was not applied")
	}
	if got := controller.state.Sessions[1].Context.Goal; got != "reviewer goal" {
		t.Fatalf("reviewer context = %q", got)
	}
	if got := controller.state.Sessions[0].Context.Goal; got != "" {
		t.Fatalf("colliding Protonman session context changed to %q", got)
	}
}

func TestInspectorResultsRejectStaleClient(t *testing.T) {
	controller := newTestController()
	current := controller.clients[ProtonmanAgentID]
	if _, _, _, _, ok := controller.beginSessionRefresh("session-1", true, contextRefreshKind, &acpclient.Client{}); ok {
		t.Fatal("stale client was admitted for inspector refresh")
	}
	controller.state.Sessions[0].Context.Memory = desktopstate.MemoryState{Global: []desktopstate.MemoryEntryState{{ID: "keep"}}}
	stale := &acpclient.Client{}
	if controller.applySessionContext(stale, sessionContextResult{SessionID: "session-1", Goal: "stale"}) {
		t.Fatal("stale context result was applied")
	}
	if controller.applySessionMemory(stale, sessionMemoryResult{SessionID: "session-1", WorkspaceKey: "stale"}) {
		t.Fatal("stale memory result was applied")
	}
	if controller.applySessionRuntime(stale, sessionRuntimeResult{SessionID: "session-1", Model: "stale"}) {
		t.Fatal("stale runtime result was applied")
	}
	if controller.applySessionSkills(stale, sessionSkillsResult{SessionID: "session-1"}) {
		t.Fatal("stale skills result was applied")
	}
	if !controller.applySessionRuntime(current, sessionRuntimeResult{SessionID: "session-1", Provider: "openai", Model: "gpt", Reasoning: "high", LowConcurrency: "on"}) {
		t.Fatal("current runtime result was rejected")
	}
	if !controller.applySessionContext(current, sessionContextResult{SessionID: "session-1", Goal: "current"}) {
		t.Fatal("current context result was rejected")
	}
	skillsResult := sessionSkillsResult{SessionID: "session-1"}
	skillsResult.Skills = append(skillsResult.Skills, struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Scope       string   `json:"scope"`
		Active      bool     `json:"active"`
		Locked      bool     `json:"locked"`
		LockStatus  string   `json:"lockStatus"`
		Resources   []string `json:"resources"`
	}{Name: "pdf", Description: "parse pdfs", Scope: "project", Active: true})
	if !controller.applySessionSkills(current, skillsResult) {
		t.Fatal("current skills result was rejected")
	}
	if got := controller.state.Sessions[0].Skills; len(got) != 1 || got[0].Name != "pdf" || !got[0].Active {
		t.Fatalf("skills update = %+v", got)
	}
	if got := controller.state.Sessions[0].Context.Memory.Global; len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("context update replaced memory: %+v", got)
	}
	if got := controller.state.Sessions[0].Runtime; got.Model != "gpt" || got.Reasoning != "high" {
		t.Fatalf("runtime state = %+v", got)
	}
	controller.state.ActiveSessionID = "another-session"
	if controller.applySessionMemory(current, sessionMemoryResult{SessionID: "session-1", WorkspaceKey: "inactive"}) {
		t.Fatal("inactive session memory result was retained")
	}
	if controller.applySessionSkills(current, sessionSkillsResult{SessionID: "session-1"}) {
		t.Fatal("inactive session skills result was retained")
	}
}

func TestRuntimeMutationRejectsBusyOrDuplicateRequests(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "session-1"
	controller.state.Sessions[0].Status = desktopstate.TaskRunning
	controller.SetRuntimeReasoning("high")
	if controller.runtimeMutation != "" {
		t.Fatal("busy session accepted a runtime mutation")
	}

	controller.state.Sessions[0].Status = desktopstate.TaskIdle
	controller.runtimeMutation = "other-session"
	controller.SetRuntimeLowConcurrency("on")
	if controller.runtimeMutation != "other-session" {
		t.Fatalf("duplicate mutation replaced active request: %q", controller.runtimeMutation)
	}
}
