package main

import (
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/session"
)

func TestApplyAgentProfileAddsDefaultCodingPrompt(t *testing.T) {
	cfg := &config.Snapshot{}
	state := &session.State{}
	if err := applyAgentProfile(cfg, state, ""); err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 1 || state.Messages[0].Role != model.RoleSystem {
		t.Fatalf("messages = %#v, want one system prompt", state.Messages)
	}
	if state.Messages[0].Content != agent.DefaultSystemPrompt() {
		t.Fatal("default coding prompt was not installed")
	}
}

func TestApplyAgentProfilePreservesExistingSystemPromptWithoutProfile(t *testing.T) {
	cfg := &config.Snapshot{}
	state := &session.State{Messages: []session.Message{{Role: model.RoleSystem, Content: "custom system"}, {Role: model.RoleUser, Content: "hello"}}}
	if err := applyAgentProfile(cfg, state, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(state.Messages[0].Content, "custom system") {
		t.Fatalf("existing system prompt was replaced: %q", state.Messages[0].Content)
	}
}
