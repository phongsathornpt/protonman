package config

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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

func TestLoadSubagentReasoningMergesFieldWise(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".proton", "config.toml"), `[agent.subagents.strength]
provider = "protonman"
model = "coding-model"
reasoning_effort = "low"

[agent.subagents.agility]
reasoning_effort = "low"
`)
	writeConfig(t, filepath.Join(workDir, ".proton", "config.toml"), `[agent.subagents.strength]
reasoning_effort = "high"

[agent.subagents.agility]
provider = "opencode"
model = "fast-model"
`)

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir, ProjectTrusted: true})
	if err != nil {
		t.Fatal(err)
	}
	strength := snapshot.Agent.Subagents["strength"]
	if strength.Provider != "protonman" || strength.Model != "coding-model" || strength.ReasoningEffort != sdk.ReasoningHigh {
		t.Fatalf("strength config = %#v, want preserved model with project reasoning override", strength)
	}
	agility := snapshot.Agent.Subagents["agility"]
	if agility.Provider != "opencode" || agility.Model != "fast-model" || agility.ReasoningEffort != sdk.ReasoningLow {
		t.Fatalf("agility config = %#v, want project model with preserved user reasoning", agility)
	}
}

func TestLoadSubagentReasoningAllowsInheritedModel(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".proton", "config.toml"), `[agent.subagents.intelligence]
reasoning_effort = "high"
`)

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	got := snapshot.Agent.Subagents["intelligence"]
	if got.Provider != "" || got.Model != "" || got.ReasoningEffort != sdk.ReasoningHigh {
		t.Fatalf("intelligence config = %#v, want inherited model with high reasoning", got)
	}
}

func TestLoadSubagentReasoningRejectsInvalidValue(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	writeConfig(t, filepath.Join(homeDir, ".proton", "config.toml"), `[agent.subagents.agility]
reasoning_effort = "turbo"
`)

	_, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: workDir})
	if err == nil || !strings.Contains(err.Error(), "agent.subagents.agility.reasoning_effort") {
		t.Fatalf("Load() error = %v, want subagent reasoning error", err)
	}
}
