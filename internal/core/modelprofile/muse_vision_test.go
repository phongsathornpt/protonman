package modelprofile

import "testing"

func TestMuseSparkVisionMatchesNamespacedIDs(t *testing.T) {
	for _, modelID := range []string{
		"muse-spark-1.3-contributor",
		"muse-spark-1.3-contributor-free",
		"router/muse-spark-1.3-contributor",
		"opencode/muse-spark-1.3-contributor-free",
	} {
		t.Run(modelID, func(t *testing.T) {
			resolved := ResolveBuiltin("gateway", modelID, CatalogMetadata{})
			if resolved.ProfileName != "muse-spark-1.3-family" {
				t.Fatalf("profile = %q, want muse-spark-1.3-family", resolved.ProfileName)
			}
			if resolved.Capabilities.Tools != SupportYes || resolved.Capabilities.Vision != SupportYes {
				t.Fatalf("capabilities = %+v, want tools and vision support", resolved.Capabilities)
			}
		})
	}
}

func TestMuseSparkKnownVariantsUseExactMetadata(t *testing.T) {
	for _, modelID := range []string{
		"muse-spark-1.3",
		"muse-spark-1.3-contributor",
		"muse-spark-1.3-contributor-free",
		"router/muse-spark-1.3-contributor-free",
	} {
		resolved := ResolveBuiltin("gateway", modelID, CatalogMetadata{})
		if resolved.ProfileMatch != MatchExact {
			t.Errorf("%s match = %q, want exact", modelID, resolved.ProfileMatch)
		}
		if resolved.Provenance.Tools != MetadataSourceBuiltin {
			t.Errorf("%s tools provenance = %q, want builtin", modelID, resolved.Provenance.Tools)
		}
	}
}
