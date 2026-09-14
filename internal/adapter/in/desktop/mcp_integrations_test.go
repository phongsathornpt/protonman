//go:build desktop

package desktop

import (
	"reflect"
	"strings"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestParseEnvironmentKeysRejectsInlineSecrets(t *testing.T) {
	_, err := parseEnvironmentKeys(`["TOKEN=secret"]`)
	if err == nil {
		t.Fatal("expected inline MCP environment secret to be rejected")
	}
	if !strings.Contains(err.Error(), "not stored") {
		t.Fatalf("error = %q, want secret-persistence guidance", err)
	}
}

func TestNormalizeIntegrationsStripsLegacyEnvironmentValues(t *testing.T) {
	items, err := normalizeIntegrations([]desktopstate.MCPIntegrationState{{
		Name:    "docs",
		Command: "mcp-docs",
		Env:     []string{" TOKEN=secret ", "TOKEN=other", "REGION"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"TOKEN", "REGION"}
	if !reflect.DeepEqual(items[0].Env, want) {
		t.Fatalf("env = %#v, want %#v", items[0].Env, want)
	}
}

func TestResolveEnvironmentInjectsValuesOnlyAtRuntime(t *testing.T) {
	lookup := func(key string) (string, bool) {
		values := map[string]string{"TOKEN": "secret", "EMPTY": ""}
		value, ok := values[key]
		return value, ok
	}
	got := resolveEnvironment([]string{"TOKEN", "MISSING", "EMPTY"}, lookup)
	want := []string{"TOKEN=secret", "EMPTY="}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved env = %#v, want %#v", got, want)
	}
}

func TestNormalizeEnvironmentKeysRejectsInvalidNames(t *testing.T) {
	if _, err := normalizeEnvironmentKeys([]string{"NOT-AN-ENV"}); err == nil {
		t.Fatal("expected invalid environment variable name to fail")
	}
}
