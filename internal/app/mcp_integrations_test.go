package app

import (
	"context"
	"reflect"
	"testing"
)

type mcpIntegrationsMemoryRepository struct {
	items []MCPIntegration
	err   error
	saved []MCPIntegration
}

func (r *mcpIntegrationsMemoryRepository) Load(context.Context) ([]MCPIntegration, error) {
	return r.items, r.err
}

func (r *mcpIntegrationsMemoryRepository) Save(_ context.Context, items []MCPIntegration) error {
	r.saved = cloneMCPIntegrations(items)
	return r.err
}

func cloneMCPIntegrations(items []MCPIntegration) []MCPIntegration {
	out := make([]MCPIntegration, len(items))
	for index, item := range items {
		out[index] = cloneMCPIntegration(item)
	}
	return out
}

func TestMCPIntegrationsNormalizeLegacyEnvironmentValues(t *testing.T) {
	repository := &mcpIntegrationsMemoryRepository{items: []MCPIntegration{{
		Name:    " docs ",
		Command: " mcp-docs ",
		Args:    []string{" --stdio ", ""},
		Env:     []string{"TOKEN=secret", "TOKEN", "REGION"},
	}}}
	integrations := NewMCPIntegrations(repository)

	items, err := integrations.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []MCPIntegration{{
		Name:    "docs",
		Command: "mcp-docs",
		Args:    []string{"--stdio"},
		Env:     []string{"TOKEN", "REGION"},
	}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("items = %#v, want %#v", items, want)
	}
	if !reflect.DeepEqual(repository.saved, want) {
		t.Fatalf("migrated items = %#v, want %#v", repository.saved, want)
	}
}

func TestMCPIntegrationsSaveCopiesAndNormalizesInput(t *testing.T) {
	repository := &mcpIntegrationsMemoryRepository{}
	integrations := NewMCPIntegrations(repository)
	items := []MCPIntegration{{Name: "z", Command: "server-z", Env: []string{"TOKEN"}}, {Name: "a", Command: "server-a"}}

	if err := integrations.Save(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	items[0].Env[0] = "changed"
	if got := repository.saved[0].Name; got != "a" {
		t.Fatalf("saved order = %#v", repository.saved)
	}
	if repository.saved[1].Env[0] != "TOKEN" {
		t.Fatalf("saved environment aliases input: %#v", repository.saved)
	}
}

func TestMCPIntegrationsSaveRejectsInvalidEnvironmentNames(t *testing.T) {
	integrations := NewMCPIntegrations(&mcpIntegrationsMemoryRepository{})
	err := integrations.Save(context.Background(), []MCPIntegration{{Name: "docs", Command: "server", Env: []string{"NOT-AN-ENV"}}})
	if err == nil {
		t.Fatal("expected invalid environment name to fail")
	}
}
