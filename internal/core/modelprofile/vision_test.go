package modelprofile

import "testing"

func TestResolveBuiltinVisionPolicies(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		modelID    string
		scheme     VisionTokenScheme
		maxDim     int
		fallback   int
	}{
		{name: "gemini tiles", provider: "google", modelID: "gemini-3.8-flash", scheme: VisionTokenGeminiTiles, maxDim: 6000, fallback: 2064},
		{name: "claude pixels", provider: "anthropic", modelID: "claude-sonnet-4-6", scheme: VisionTokenAnthropicPixels, maxDim: 1568, fallback: 1600},
		{name: "responses patch fallback", provider: "openai", modelID: "muse-spark-1.3-contributor", scheme: VisionTokenPatch32, maxDim: 2048, fallback: 2500},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolved := ResolveBuiltin(tc.provider, tc.modelID, CatalogMetadata{})
			policy := EffectiveVisionPolicy(resolved)
			if policy.TokenScheme != tc.scheme {
				t.Fatalf("token scheme = %q, want %q", policy.TokenScheme, tc.scheme)
			}
			if policy.MaxDimension != tc.maxDim {
				t.Fatalf("max dimension = %d, want %d", policy.MaxDimension, tc.maxDim)
			}
			if policy.FallbackTokens != tc.fallback {
				t.Fatalf("fallback tokens = %d, want %d", policy.FallbackTokens, tc.fallback)
			}
		})
	}
}

func TestResolveBuiltinMuseSpark13SupportsVision(t *testing.T) {
	resolved := ResolveBuiltin("openai", "muse-spark-1.3-contributor", CatalogMetadata{})
	if resolved.Capabilities.Vision != SupportYes {
		t.Fatalf("vision support = %v, want %v", resolved.Capabilities.Vision, SupportYes)
	}
	if resolved.Provenance.Vision != MetadataSourceBuiltin {
		t.Fatalf("vision provenance = %v, want %v", resolved.Provenance.Vision, MetadataSourceBuiltin)
	}
}

func TestCatalogCanDisableBuiltinMuseSparkVision(t *testing.T) {
	disabled := false
	resolved := ResolveBuiltin("openai", "muse-spark-1.3-contributor", CatalogMetadata{Vision: &disabled})
	if resolved.Capabilities.Vision != SupportNo {
		t.Fatalf("vision support = %v, want %v", resolved.Capabilities.Vision, SupportNo)
	}
	if resolved.Provenance.Vision != MetadataSourceCatalog {
		t.Fatalf("vision provenance = %v, want %v", resolved.Provenance.Vision, MetadataSourceCatalog)
	}
}

func TestEffectiveVisionPolicyFallsBackConservatively(t *testing.T) {
	got := EffectiveVisionPolicy(Resolved{})
	want := DefaultVisionPolicy()
	if got != want {
		t.Fatalf("effective policy = %+v, want %+v", got, want)
	}
}

func TestRegistryRejectsInvalidVisionPolicy(t *testing.T) {
	_, err := NewRegistry(Profile{
		Name:  "bad-vision",
		Match: Matcher{Prefixes: []string{"bad"}},
		VisionPolicy: VisionPolicy{
			TokenScheme: "made_up",
		},
	})
	if err == nil {
		t.Fatal("NewRegistry() error = nil, want invalid vision token scheme")
	}
}
