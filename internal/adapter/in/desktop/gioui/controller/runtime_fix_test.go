//go:build desktop || desktop_gio

package controller

import (
	"strings"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

// Cross-provider model switches must go through the provider-aware runtime
// method, not the model-only config-option fast path. The fast path validates
// the model against the *current* provider, so it rejects any model from a
// different provider as "not an advertised session config value".
func TestSetRuntimeModelSwitchesProviderThroughLegacyMethod(t *testing.T) {
	c := newTestController()
	c.state.ActiveSessionID = "session-1"
	c.state.Sessions[0].Runtime.Provider = "openai"
	c.state.Sessions[0].Runtime.Model = "gpt-old"

	c.SetRuntimeModel("anthropic", "claude-new")

	if c.runtimeMutation == "" {
		t.Fatal("provider switch did not start a runtime mutation")
	}
	c.Close()
}

func TestSetRuntimeModelSameProviderKeepsFastPath(t *testing.T) {
	c := newTestController()
	c.state.ActiveSessionID = "session-1"
	c.state.Sessions[0].Runtime.Provider = "openai"
	c.state.Sessions[0].Runtime.Model = "gpt-old"

	c.SetRuntimeModel("openai", "gpt-new")

	if c.runtimeMutation == "" {
		t.Fatal("same-provider model change did not start a runtime mutation")
	}
	c.Close()
}

func TestStartRuntimeMutationSurfacesBlockedReason(t *testing.T) {
	c := newTestController()
	c.state.ActiveSessionID = ""
	c.startRuntimeMutation("protonman/session/set_reasoning", map[string]any{"reasoning": "high"})
	snap := c.Snapshot()
	if !strings.HasPrefix(snap.Status, "Runtime not updated") {
		t.Fatalf("status = %q, want Runtime not updated prefix", snap.Status)
	}
	c.Close()
}

func TestEqualProviderIDIgnoresCase(t *testing.T) {
	if !equalProviderID("OpenAI", "openai") {
		t.Fatal("provider comparison must be case-insensitive")
	}
	if equalProviderID("openai", "anthropic") {
		t.Fatal("different providers must not compare equal")
	}
}

func TestFetchProviderModelsNormalizesLoadingKey(t *testing.T) {
	c := newTestController()
	_ = desktopstate.TaskIdle
	c.FetchProviderModels("OpenAI", "", "", "", nil)
	snap := c.Snapshot()
	if !snap.ProviderModelsLoading["openai"] {
		t.Fatalf("loading keys = %#v, want lower-cased openai", snap.ProviderModelsLoading)
	}
	if snap.ProviderModelsLoading["OpenAI"] {
		t.Fatalf("loading keys = %#v, must not keep raw case", snap.ProviderModelsLoading)
	}
	c.Close()
}
