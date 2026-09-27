package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app"
)

func TestUserACPAgentsStoreRoundTrip(t *testing.T) {
	home := t.TempDir()
	store := NewUserACPAgentsStore(home)
	want := []app.ACPAgentProfile{{
		ID:          "reviewer",
		DisplayName: "Reviewer",
		Command:     "reviewer-acp",
		Args:        []string{"--stdio"},
		Env:         []string{"TOKEN"},
	}}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != want[0].ID || got[0].Command != want[0].Command || got[0].Env[0] != "TOKEN" {
		t.Fatalf("loaded profiles = %#v", got)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".protonman", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), acpAgentsPreferencesKey) {
		t.Fatalf("configuration is missing %q: %s", acpAgentsPreferencesKey, raw)
	}
}

func TestUserACPAgentsStoreRejectsInlineEnvironmentValues(t *testing.T) {
	store := NewUserACPAgentsStore(t.TempDir())
	err := store.Save(context.Background(), []app.ACPAgentProfile{{ID: "custom", Command: "agent", Env: []string{"TOKEN=secret"}}})
	if err == nil || !strings.Contains(err.Error(), "not persisted") {
		t.Fatalf("error = %v, want environment persistence rejection", err)
	}
}

func TestUserACPAgentsStoreMigratesLegacyEnvironmentValues(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".protonman")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	legacy := `{"preferences":{"acp.agents.v1":[{"id":"custom","command":"agent","env":["TOKEN=secret"]}]}}`
	if err := os.WriteFile(configPath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	agents := app.NewACPAgents(NewUserACPAgentsStore(home))
	profiles, err := agents.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || len(profiles[0].Env) != 1 || profiles[0].Env[0] != "TOKEN" {
		t.Fatalf("migrated profiles = %#v", profiles)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || !strings.Contains(string(raw), `"TOKEN"`) {
		t.Fatalf("migrated configuration = %s", raw)
	}
}

func TestUserACPAgentsStoreDecodesLegacyStringPreference(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".protonman")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := `{"preferences":{"acp.agents.v1":"[{\"id\":\"custom\",\"command\":\"agent\"}]"}}`
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	profiles, err := NewUserACPAgentsStore(home).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ID != "custom" {
		t.Fatalf("profiles = %#v", profiles)
	}
}
