package config

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSubagentModelsDefaultsToInherit(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Agent.Subagents == nil {
		t.Fatal("subagent model map is nil")
	}
	if len(snapshot.Agent.Subagents) != 0 {
		t.Fatalf("default subagent models = %#v, want dynamic inheritance", snapshot.Agent.Subagents)
	}
}

func TestLoadSubagentModelsMergePerProfile(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".proton", "config.toml"), `[agent.subagents.strength]
provider = "protonman"
model = "coding-model"

[agent.subagents.agility]
provider = "opencode"
model = "fast-model"
`)
	writeConfig(t, filepath.Join(workDir, ".proton", "config.toml"), `[agent.subagents.intelligence]
provider = "anthropic"
model = "reasoning-model"

[agent.subagents.strength]
provider = "custom"
model = "project-coding-model"
`)

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir, ProjectTrusted: true})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]SubagentModelConfig{
		"strength":     {Provider: "custom", Model: "project-coding-model"},
		"agility":      {Provider: "opencode", Model: "fast-model"},
		"intelligence": {Provider: "anthropic", Model: "reasoning-model"},
	}
	if len(snapshot.Agent.Subagents) != len(want) {
		t.Fatalf("subagent models = %#v, want %#v", snapshot.Agent.Subagents, want)
	}
	for profile, expected := range want {
		if got := snapshot.Agent.Subagents[profile]; got != expected {
			t.Fatalf("subagent model %s = %#v, want %#v", profile, got, expected)
		}
	}
}

func TestLoadSubagentModelsRejectsIncompleteOverride(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".proton", "config.toml"), `[agent.subagents.strength]
provider = "protonman"
`)

	_, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err == nil || !strings.Contains(err.Error(), "provider and model must both be set") {
		t.Fatalf("Load() error = %v, want incomplete subagent model error", err)
	}
}

func TestLoadSubagentModelsRejectsUnsupportedProfile(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".proton", "config.toml"), `[agent.subagents.universal]
provider = "protonman"
model = "main-model"
`)

	_, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err == nil || !strings.Contains(err.Error(), "unsupported profile") {
		t.Fatalf("Load() error = %v, want unsupported subagent profile error", err)
	}
}

func TestLoadSubagentModelsRejectsLegacyAlias(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".proton", "config.toml"), `[agent.subagents.pow]
provider = "protonman"
model = "legacy-model"
`)

	_, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err == nil || !strings.Contains(err.Error(), "unsupported profile") {
		t.Fatalf("Load() error = %v, want legacy profile rejection", err)
	}
}
