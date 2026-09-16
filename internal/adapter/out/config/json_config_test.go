package config

import (
	"context"
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
