package model

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
)

func TestOpenCodeInferenceDiscoveryDoesNotProbeModels(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	models, err := FetchProviderModelsForProtocol(
		context.Background(),
		ProviderProtocolOpenAI,
		server.URL+"/inference/openai/v1",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatalf("inference discovery made %d HTTP requests, want 0", requests.Load())
	}
	if len(models) != 3 || models[2].ID != DefaultOpenCodeModel {
		t.Fatalf("models = %#v", models)
	}
}

func TestOpenCodeInferenceCatalogIsCopied(t *testing.T) {
	first := OpenCodeInferenceFreeModels()
	first[0].ID = "mutated"
	second := OpenCodeInferenceFreeModels()
	if second[0].ID == "mutated" {
		t.Fatal("OpenCode inference catalog exposed mutable shared state")
	}
}

func TestOpenCodeZenRequiresKeyBeforeDiscovery(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := FetchProviderModelsForProtocol(
		context.Background(),
		ProviderProtocolOpenAI,
		server.URL+"/zen/v1",
		"",
	)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("unauthenticated Zen discovery made %d requests", requests.Load())
	}
}

func TestOpenCodeZenDiscoveryUsesBearerAndFiltersUnsupportedModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zen/v1/models" {
			t.Fatalf("path = %q, want /zen/v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer zen-key" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.5"},{"id":"claude-sonnet-5"},{"id":"gemini-3.8-flash"}]}`))
	}))
	defer server.Close()

	models, err := FetchProviderModelsForProtocol(
		context.Background(),
		ProviderProtocolOpenAI,
		server.URL+"/zen/v1",
		"zen-key",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "gpt-5.5" || models[1].ID != "claude-sonnet-5" {
		t.Fatalf("filtered models = %#v", models)
	}
}

func TestMatchProviderPresetDistinguishesOpenCodeRoutes(t *testing.T) {
	tests := []struct {
		name    string
		preset  string
		baseURL string
		keyless bool
	}{
		{name: "inference", preset: DefaultOpenCodeName, baseURL: OpenCodeInferenceEndpoint, keyless: true},
		{name: "zen", preset: "opencode-zen", baseURL: OpenCodeZenEndpoint, keyless: false},
		{name: "go", preset: "opencode-go", baseURL: OpenCodeGoEndpoint, keyless: false},
		{name: "local zen", preset: "opencode-zen", baseURL: "http://127.0.0.1:20128/zen/v1", keyless: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preset := MatchProviderPreset(DefaultOpenCodeName, test.baseURL)
			if preset == nil || preset.ID != test.preset {
				t.Fatalf("preset = %#v, want %q", preset, test.preset)
			}
			if got := !preset.RequiresKey; got != test.keyless {
				t.Fatalf("RequiresKey = %v, want keyless=%v", preset.RequiresKey, test.keyless)
			}
		})
	}

	if ProviderHasUsableAuth(DefaultOpenCodeName, OpenCodeZenEndpoint, "") {
		t.Fatal("anonymous Zen route must not be usable")
	}
	if !ProviderHasUsableAuth(DefaultOpenCodeName, OpenCodeZenEndpoint, "zen-key") {
		t.Fatal("keyed Zen route should be usable")
	}
	if !strings.Contains(MatchProviderPreset(DefaultOpenCodeName, OpenCodeZenEndpoint).Description, "model-specific") {
		t.Fatal("Zen preset description does not explain transport-specific behavior")
	}
}

func TestResolvePrimaryModelDefaultsMigratesAnonymousZenConfig(t *testing.T) {
	providers := map[string]modelconfig.Provider{
		DefaultOpenCodeName: {
			Name:    DefaultOpenCodeName,
			Type:    string(ProviderProtocolOpenAI),
			BaseURL: OpenCodeZenEndpoint,
		},
	}
	selection, providers := ResolvePrimaryModelDefaults(modelconfig.Selection{
		Provider: DefaultOpenCodeName,
		Default:  "nemotron-3.5-lightning-free",
	}, providers)
	if selection.Default != DefaultOpenCodeModel {
		t.Fatalf("default model = %q, want %q", selection.Default, DefaultOpenCodeModel)
	}
	if providers[DefaultOpenCodeName].BaseURL != DefaultOpenCodeEndpoint {
		t.Fatalf("base URL = %q, want %q", providers[DefaultOpenCodeName].BaseURL, DefaultOpenCodeEndpoint)
	}
}

func TestResolvePrimaryModelDefaultsPreservesKeyedZenConfig(t *testing.T) {
	providers := map[string]modelconfig.Provider{
		DefaultOpenCodeName: {
			Name:    DefaultOpenCodeName,
			Type:    string(ProviderProtocolOpenAI),
			BaseURL: OpenCodeZenEndpoint,
			APIKey:  "zen-key",
		},
	}
	selection, providers := ResolvePrimaryModelDefaults(modelconfig.Selection{
		Provider: DefaultOpenCodeName,
		Default:  "gpt-5.5",
	}, providers)
	if selection.Default != "gpt-5.5" || providers[DefaultOpenCodeName].BaseURL != OpenCodeZenEndpoint {
		t.Fatalf("keyed Zen config changed: selection=%#v provider=%#v", selection, providers[DefaultOpenCodeName])
	}
}
