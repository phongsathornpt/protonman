package main

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/session"
)

func TestApplyAgentProfileLeavesDefaultTranscriptUnmanaged(t *testing.T) {
	cfg := &config.Snapshot{}
	state := &session.State{}
	if err := applyAgentProfile(cfg, state, ""); err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 0 {
		t.Fatalf("messages = %#v, want no generated system prompt", state.Messages)
	}
	if state.AgentProfile != "" || cfg.Agent.Profile != "" {
		t.Fatalf("profile state=%q config=%q, want default", state.AgentProfile, cfg.Agent.Profile)
	}
}

func TestApplyAgentProfilePreservesCustomSystemInstruction(t *testing.T) {
	cfg := &config.Snapshot{}
	state := &session.State{Messages: []session.Message{
		{Role: model.RoleSystem, Content: "custom system"},
		{Role: model.RoleUser, Content: "hello"},
	}}
	if err := applyAgentProfile(cfg, state, "dex"); err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 2 || state.Messages[0].Content != "custom system" {
		t.Fatalf("custom transcript changed: %#v", state.Messages)
	}
	if state.AgentProfile != "intelligence" || cfg.Agent.Profile != "intelligence" {
		t.Fatalf("profile state=%q config=%q, want intelligence", state.AgentProfile, cfg.Agent.Profile)
	}
}

func TestApplyAgentProfilePrecedence(t *testing.T) {
	cfg := &config.Snapshot{}
	cfg.Agent.Profile = "worker"
	state := &session.State{AgentProfile: "dex"}
	if err := applyAgentProfile(cfg, state, "pow"); err != nil {
		t.Fatal(err)
	}
	if state.AgentProfile != "strength" || cfg.Agent.Profile != "strength" {
		t.Fatalf("explicit profile did not win: state=%q config=%q", state.AgentProfile, cfg.Agent.Profile)
	}

	cfg.Agent.Profile = "worker"
	state.AgentProfile = "dex"
	if err := applyAgentProfile(cfg, state, ""); err != nil {
		t.Fatal(err)
	}
	if state.AgentProfile != "intelligence" || cfg.Agent.Profile != "intelligence" {
		t.Fatalf("session profile did not win: state=%q config=%q", state.AgentProfile, cfg.Agent.Profile)
	}
}
