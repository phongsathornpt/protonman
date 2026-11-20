//go:build desktop || desktop_gio

package controller

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestInspectorResultAppliesToProtonSession(t *testing.T) {
	controller := newTestController()
	reviewer := &acpclient.Client{}
	_ = reviewer

	controller.state.ActiveSessionID = "session-1"
	controller.state.ActiveAgentID = ProtonmanAgentID

	if !controller.applySessionContext(controller.client, sessionContextResult{SessionID: "session-1", Goal: "proton goal"}, ProtonmanAgentID) {
		t.Fatal("proton context result was not applied")
	}
	if got := controller.state.Sessions[0].Context.Goal; got != "proton goal" {
		t.Fatalf("proton context = %q", got)
	}
}

func TestInspectorResultsRejectStaleClient(t *testing.T) {
	controller := newTestController()
	current := controller.client
	if _, _, _, _, ok := controller.beginSessionRefresh("session-1", true, contextRefreshKind, &acpclient.Client{}); ok {
		t.Fatal("stale client was admitted for inspector refresh")
	}
	controller.state.Sessions[0].Context.Memory = desktopstate.MemoryState{Global: []desktopstate.MemoryEntryState{{ID: "keep"}}}
	stale := &acpclient.Client{}
	if controller.applySessionContext(stale, sessionContextResult{SessionID: "session-1", Goal: "stale"}, ProtonmanAgentID) {
		t.Fatal("stale context result was applied")
	}
	if controller.applySessionMemory(stale, sessionMemoryResult{SessionID: "session-1", WorkspaceKey: "stale"}, ProtonmanAgentID) {
		t.Fatal("stale memory result was applied")
	}
	if controller.applySessionRuntime(stale, sessionRuntimeResult{SessionID: "session-1", Model: "stale"}, ProtonmanAgentID) {
		t.Fatal("stale runtime result was applied")
	}
	if controller.applySessionSkills(stale, sessionSkillsResult{SessionID: "session-1"}, ProtonmanAgentID) {
		t.Fatal("stale skills result was applied")
	}
	if !controller.applySessionRuntime(current, sessionRuntimeResult{SessionID: "session-1", Provider: "openai", Model: "gpt", Reasoning: "high", LowConcurrency: "on"}, ProtonmanAgentID) {
		t.Fatal("current runtime result was rejected")
	}
	if !controller.applySessionContext(current, sessionContextResult{SessionID: "session-1", Goal: "current"}, ProtonmanAgentID) {
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
	if !controller.applySessionSkills(current, skillsResult, ProtonmanAgentID) {
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
	if controller.applySessionMemory(current, sessionMemoryResult{SessionID: "session-1", WorkspaceKey: "inactive"}, ProtonmanAgentID) {
		t.Fatal("inactive session memory result was retained")
	}
	if controller.applySessionSkills(current, sessionSkillsResult{SessionID: "session-1"}, ProtonmanAgentID) {
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
