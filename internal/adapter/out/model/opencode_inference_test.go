package model

import (
	"context"
	"strings"
	"testing"
)

func TestOpenCodeFreePresetUsesInferenceAPI(t *testing.T) {
	if DefaultOpenCodeEndpoint != "https://opencode.ai/inference/openai/v1" {
		t.Fatalf("DefaultOpenCodeEndpoint = %q", DefaultOpenCodeEndpoint)
	}
	if DefaultOpenCodeModel != "nemotron-3-super-free" {
		t.Fatalf("DefaultOpenCodeModel = %q", DefaultOpenCodeModel)
	}
	preset := MatchProviderPreset(DefaultOpenCodeName, DefaultOpenCodeEndpoint)
	if preset == nil || preset.ID != DefaultOpenCodeName || preset.RequiresKey {
		t.Fatalf("free preset = %#v", preset)
	}
}

func TestOpenCodeZenIsNotAnonymousFreePreset(t *testing.T) {
	if preset := MatchProviderPreset(DefaultOpenCodeName, OpenCodeZenEndpoint); preset != nil {
		t.Fatalf("Zen endpoint must not match keyless free preset: %#v", preset)
	}
	if ProviderHasUsableAuth(DefaultOpenCodeName, OpenCodeZenEndpoint, "") {
		t.Fatal("Zen endpoint must not be usable anonymously")
	}
	if !ProviderHasUsableAuth(DefaultOpenCodeName, OpenCodeZenEndpoint, "zen-key") {
		t.Fatal("explicitly keyed Zen endpoint should remain usable as a custom route")
	}
}

func TestOpenCodeInferenceDiscoveryUsesDocumentedFreeCatalog(t *testing.T) {
	models, err := FetchProviderModelsForProtocol(
		context.Background(), ProviderProtocolOpenAI, DefaultOpenCodeEndpoint, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"big-pickle":            false,
		"mimo-v2.5-free":        false,
		"nemotron-3-super-free": false,
	}
	for _, model := range models {
		if _, ok := want[model.ID]; ok {
			want[model.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Fatalf("documented free model %q missing from %#v", id, models)
		}
	}
}

func TestOpenCodeClientIdentityUsesProtonman(t *testing.T) {
	cfg := newClientConfig(DefaultOpenCodeEndpoint, "", DefaultOpenCodeModel)
	if cfg.clientName != "protonman" {
		t.Fatalf("clientName = %q", cfg.clientName)
	}
	if !strings.HasPrefix(strings.ToLower(cfg.userAgent), "protonman/") {
		t.Fatalf("userAgent = %q", cfg.userAgent)
	}
}
