package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/app"
)

func TestUserMCPIntegrationsStoreRoundTrip(t *testing.T) {
	home := t.TempDir()
	store := NewUserMCPIntegrationsStore(home)
	want := []app.MCPIntegration{{
		Name:    "docs",
		Command: "mcp-docs",
		Args:    []string{"--stdio"},
		Env:     []string{"TOKEN"},
	}}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != want[0].Name || got[0].Command != want[0].Command {
		t.Fatalf("loaded integrations = %#v", got)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".protonman", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("configuration contains an environment secret: %s", raw)
	}
	if !strings.Contains(string(raw), mcpIntegrationsPreferencesKey) {
		t.Fatalf("configuration is missing %q: %s", mcpIntegrationsPreferencesKey, raw)
	}
}

func TestUserMCPIntegrationsStoreRejectsInlineEnvironmentValues(t *testing.T) {
	store := NewUserMCPIntegrationsStore(t.TempDir())
	err := store.Save(context.Background(), []app.MCPIntegration{{
		Name:    "docs",
		Command: "mcp-docs",
		Env:     []string{"TOKEN=secret"},
	}})
	if err == nil || !strings.Contains(err.Error(), "not persisted") {
		t.Fatalf("error = %v, want environment persistence rejection", err)
	}
}

func TestUserMCPIntegrationsStoreMigratesLegacyEnvironmentValues(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".protonman")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	legacy := `{"preferences":{"mcp.integrations.v1":[{"name":"docs","command":"mcp-docs","env":["TOKEN=secret"]}]}}`
	if err := os.WriteFile(configPath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	integrations := app.NewMCPIntegrations(NewUserMCPIntegrationsStore(home))
	items, err := integrations.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || len(items[0].Env) != 1 || items[0].Env[0] != "TOKEN" {
		t.Fatalf("migrated items = %#v", items)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || !strings.Contains(string(raw), `"TOKEN"`) {
		t.Fatalf("migrated configuration = %s", raw)
	}
}
