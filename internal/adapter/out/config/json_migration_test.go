package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadJSONConfig(t *testing.T) {
	homeDir := t.TempDir()
	jsonContent := `{
  "permission": {
    "default": "ask",
    "rules": [
      {
        "action": "allow",
        "tool": "read",
        "pattern": "*"
      }
    ]
  },
  "agent": {
    "profile": "strength",
    "max_tool_calls": 25
  },
  "skills": {
    "active": ["test-skill"]
  }
}`
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.json"), jsonContent)

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: homeDir})
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if snapshot.Agent.Profile != "strength" {
		t.Errorf("Agent.Profile = %q, want %q", snapshot.Agent.Profile, "strength")
	}
	if snapshot.Agent.MaxToolCalls != 25 {
		t.Errorf("Agent.MaxToolCalls = %d, want 25", snapshot.Agent.MaxToolCalls)
	}
	if len(snapshot.Skills.Active) != 1 || snapshot.Skills.Active[0] != "test-skill" {
		t.Errorf("Skills.Active = %v, want [test-skill]", snapshot.Skills.Active)
	}
	if len(snapshot.Permission.Rules) != 1 || snapshot.Permission.Rules[0].Tool != "read" {
		t.Errorf("Permission.Rules = %v, want 1 rule for read", snapshot.Permission.Rules)
	}
}

func TestJSONConfigTakesPrecedenceOverTOML(t *testing.T) {
	homeDir := t.TempDir()
	tomlContent := `[agent]
profile = "agility"
`
	jsonContent := `{
  "agent": {
    "profile": "intelligence"
  }
}`
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.toml"), tomlContent)
	writeConfig(t, filepath.Join(homeDir, ".protonman", "config.json"), jsonContent)

	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: homeDir})
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if snapshot.Agent.Profile != "intelligence" {
		t.Errorf("Agent.Profile = %q, want %q (JSON should take precedence over TOML)", snapshot.Agent.Profile, "intelligence")
	}
}

func TestSaveUserConfigMigratesLegacyTOMLToJSON(t *testing.T) {
	homeDir := t.TempDir()
	tomlPath := filepath.Join(homeDir, ".protonman", "config.toml")
	jsonPath := filepath.Join(homeDir, ".protonman", "config.json")
	bakPath := filepath.Join(homeDir, ".protonman", "config.toml.bak")

	initialTOML := `[agent]
profile = "strength"
subagents_enabled = true
`
	writeConfig(t, tomlPath, initialTOML)

	// Mutate user config by saving a new setting
	if err := SaveUserMaxToolCalls(homeDir, 42); err != nil {
		t.Fatalf("SaveUserMaxToolCalls failed: %v", err)
	}

	// Verify config.json was created
	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("expected config.json to exist after save: %v", err)
	}

	// Verify legacy config.toml was backed up to config.toml.bak
	if _, err := os.Stat(bakPath); err != nil {
		t.Fatalf("expected config.toml.bak to exist after migration: %v", err)
	}

	// Verify the migrated content in JSON retained original values and applied new ones
	snapshot, err := Load(context.Background(), Options{HomeDir: homeDir, WorkDir: homeDir})
	if err != nil {
		t.Fatalf("Load() after migration failed: %v", err)
	}
	if snapshot.Agent.Profile != "strength" {
		t.Errorf("Agent.Profile = %q, want %q", snapshot.Agent.Profile, "strength")
	}
	if snapshot.Agent.MaxToolCalls != 42 {
		t.Errorf("Agent.MaxToolCalls = %d, want 42", snapshot.Agent.MaxToolCalls)
	}
}
