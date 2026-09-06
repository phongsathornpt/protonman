package main

import (
	"testing"

	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/session"
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
	if state.AgentProfile != "dex" || cfg.Agent.Profile != "dex" {
		t.Fatalf("profile state=%q config=%q, want dex", state.AgentProfile, cfg.Agent.Profile)
	}
}

func TestApplyAgentProfilePrecedence(t *testing.T) {
	cfg := &config.Snapshot{}
	cfg.Agent.Profile = "worker"
	state := &session.State{AgentProfile: "dex"}
	if err := applyAgentProfile(cfg, state, "pow"); err != nil {
		t.Fatal(err)
	}
	if state.AgentProfile != "pow" || cfg.Agent.Profile != "pow" {
		t.Fatalf("explicit profile did not win: state=%q config=%q", state.AgentProfile, cfg.Agent.Profile)
	}

	cfg.Agent.Profile = "worker"
	state.AgentProfile = "dex"
	if err := applyAgentProfile(cfg, state, ""); err != nil {
		t.Fatal(err)
	}
	if state.AgentProfile != "dex" || cfg.Agent.Profile != "dex" {
		t.Fatalf("session profile did not win: state=%q config=%q", state.AgentProfile, cfg.Agent.Profile)
	}
}
