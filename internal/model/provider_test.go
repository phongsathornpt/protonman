package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchProviderModelsOpenAIFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"gpt-4o","name":"GPT-4o"},{"id":"o3-mini","name":"o3-mini"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	models, err := FetchProviderModels(context.Background(), ts.URL, "test-key")
	if err != nil {
		t.Fatalf("FetchProviderModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "gpt-4o" || models[1].ID != "o3-mini" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestFetchProviderModelsProtonmanFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/public/models" || r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"models": [
					{
						"id": "1",
						"slug": "deepseek-v4-flash-vision-exp",
						"name": "DeepSeek V4 Flash Vision",
						"contextWindow": 1000000,
						"features": ["vision", "tools"],
						"provider": {"name": "DeepSeek"}
					}
				]
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	models, err := FetchProviderModels(context.Background(), ts.URL, "any-key")
	if err != nil {
		t.Fatalf("FetchProviderModels() error = %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ID != "deepseek-v4-flash-vision-exp" || models[0].ContextWindow != 1000000 {
		t.Fatalf("unexpected model: %+v", models[0])
	}
}

func TestFetchProviderModelsUnauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid key", http.StatusUnauthorized)
	}))
	defer ts.Close()

	_, err := FetchProviderModels(context.Background(), ts.URL, "bad-key")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got: %v", err)
	}
}

func TestFetchProviderModelsFallbackDefaultProtonman(t *testing.T) {
	models, err := FetchProviderModels(context.Background(), "https://invalid-nonexistent.protonman.dev/api/v1", "key")
	if err != nil {
		t.Fatalf("expected fallback default catalog, got error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected default models catalog")
	}
	if models[0].ID != "deepseek-v4-flash-vision-exp" {
		t.Fatalf("unexpected first model: %+v", models[0])
	}
}

func TestIsFreeModel(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"nemotron-3.5-lightning-free", true},
		{"big-pickle", true},
		{"mimo-v2.5-free", true},
		{"deepseek-v4-flash-free", true},
		{"muse-spark-1.3-contributor-free", true},
		{"NEMOTRON-3-ULTRA-FREE", true},
		{"gpt-5.5", false},
		{"claude-sonnet-5", false},
		{"deepseek-v4-pro", false},
		{"", false},
	}

	for _, tc := range cases {
		if got := IsFreeModel(tc.id); got != tc.want {
			t.Errorf("IsFreeModel(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

func TestFetchProviderModelsOpenCodeNoAuth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("expected no authorization header for empty key, got: %s", r.Header.Get("Authorization"))
		}
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"object":"list","data":[{"id":"nemotron-3.5-lightning-free","object":"model"},{"id":"big-pickle","object":"model"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	models, err := FetchProviderModels(context.Background(), ts.URL, "")
	if err != nil {
		t.Fatalf("FetchProviderModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "nemotron-3.5-lightning-free" || models[1].ID != "big-pickle" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestFetchProviderModelsFallbackDefaultOpenCode(t *testing.T) {
	models, err := FetchProviderModels(context.Background(), "https://invalid-nonexistent.opencode.ai/zen/v1", "")
	if err != nil {
		t.Fatalf("expected fallback default catalog, got error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected default models catalog")
	}
	if models[0].ID != "nemotron-3.5-lightning-free" {
		t.Fatalf("unexpected first model: %+v", models[0])
	}
}

func TestSupportedPresetsAndLookup(t *testing.T) {
	if len(SupportedPresets) < 4 {
		t.Fatalf("expected at least 4 presets, got %d", len(SupportedPresets))
	}
	oc := LookupPreset("opencode")
	if oc == nil || oc.ID != DefaultOpenCodeName {
		t.Fatalf("expected opencode preset, got %+v", oc)
	}
	if oc.RequiresKey {
		t.Fatal("expected opencode to not require key")
	}

	pm := LookupPreset("Protonman")
	if pm == nil || pm.ID != DefaultProtonmanName {
		t.Fatalf("expected protonman preset, got %+v", pm)
	}
	if !pm.RequiresKey {
		t.Fatal("expected protonman to require key")
	}

	ol := LookupPreset("ollama")
	if ol == nil || ol.RequiresKey {
		t.Fatalf("expected ollama preset without required key, got %+v", ol)
	}

	oa := LookupPreset("openai")
	if oa == nil || !oa.RequiresKey {
		t.Fatalf("expected openai preset with required key, got %+v", oa)
	}

	none := LookupPreset("non-existent")
	if none != nil {
		t.Fatalf("expected nil for unknown preset, got %+v", none)
	}
}

func TestNormalizeModelID(t *testing.T) {
	tests := []struct {
		provider string
		modelID  string
		expected string
	}{
		// Typo correction
		{"opencode", "muse-spark-1.3-contributer", "muse-spark-1.3-contributor-free"},
		{"opencode", "muse-spark-1.3-contributor", "muse-spark-1.3-contributor-free"},
		{"opencode", "muse-spark-1.3-contributor-free", "muse-spark-1.3-contributor-free"},
		{"opencode", "ling-3.0-flash-fin", "ling-3.0-flash-fin-free"},
		{"https://opencode.ai/zen/v1", "nemotron-3.5-lightning", "nemotron-3.5-lightning-free"},
		{"https://opencode.ai/zen/v1", "big-pickle", "big-pickle"},
		// Non-opencode provider preserves original name (only fixes typo if present)
		{"protonman", "muse-spark-1.3-contributer", "muse-spark-1.3-contributor"},
		{"openai", "gpt-4o", "gpt-4o"},
		{"", "", ""},
	}

	for _, tc := range tests {
		got := NormalizeModelID(tc.provider, tc.modelID)
		if got != tc.expected {
			t.Errorf("NormalizeModelID(%q, %q) = %q, want %q", tc.provider, tc.modelID, got, tc.expected)
		}
	}
}



