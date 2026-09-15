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
			if resolved.Capabilities.Vision != SupportYes {
				t.Fatalf("vision support = %v, want %v", resolved.Capabilities.Vision, SupportYes)
			}
		})
	}
}
