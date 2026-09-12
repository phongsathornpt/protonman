package modelprofile

import "testing"

func TestRegistryDiscardsLegacyPromptHints(t *testing.T) {
	registry, err := NewRegistry(Profile{
		Name:  "legacy-prompt-policy",
		Match: Matcher{Provider: "test"},
		AgentPolicy: AgentPolicy{
			PromptHints: []string{"provider-specific natural-language guidance"},
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	resolved := registry.Resolve("test", "model", CatalogMetadata{})
	if len(resolved.AgentPolicy.PromptHints) != 0 {
		t.Fatalf("resolved prompt hints = %v, want none", resolved.AgentPolicy.PromptHints)
	}
	if resolved.Provenance.PromptHints != MetadataSourceUnknown {
		t.Fatalf("prompt hint provenance = %q, want unknown", resolved.Provenance.PromptHints)
	}
}
