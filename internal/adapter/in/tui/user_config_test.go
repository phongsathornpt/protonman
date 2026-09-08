package tui

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestUserConfigSetSubagentsPersistsAndApplies(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("PROTON_HOME", homeDir)
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.workDir = workDir
	m.agents = app.NewAgents(coord)
	m.subagentsEnabled = true

	cmd := m.executeCommand("/config set subagents off")
	if cmd == nil {
		t.Fatal("config command returned nil")
	}
	updated, _ := m.Update(cmd())
	m = updated.(*bubbleModel)
	if m.subagentsEnabled || coord.Enabled() {
		t.Fatal("user config did not disable live subagents")
	}
	if got := m.projectSource(config.FieldAgentSubagentsEnabled); got != config.SourceUser {
		t.Fatalf("provenance = %q, want user", got)
	}
	snapshot, err := config.Load(context.Background(), config.Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent.SubagentsEnabled {
		t.Fatal("user config did not persist subagents_enabled=false")
	}
}

func TestUserConfigDoesNotOverrideTrustedProjectSetting(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("PROTON_HOME", homeDir)
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.workDir = workDir
	m.agents = app.NewAgents(coord)
	m.subagentsEnabled = true
	m.projectConfigProvenance = map[string]config.ValueSource{
		config.FieldAgentSubagentsEnabled: config.SourceProject,
	}

	cmd := m.executeCommand("/config set subagents off")
	updated, _ := m.Update(cmd())
	m = updated.(*bubbleModel)
	if !m.subagentsEnabled || !coord.Enabled() {
		t.Fatal("user config incorrectly overrode trusted project runtime setting")
	}
	snapshot, err := config.Load(context.Background(), config.Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent.SubagentsEnabled {
		t.Fatal("user default was not persisted")
	}
}
