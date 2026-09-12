package projectconfig

import "github.com/phongsathornpt/protonman/internal/adapter/out/config"

// CloneProvenance snapshots project config provenance before crossing UI ownership boundaries.
func CloneProvenance(in map[string]config.ValueSource) map[string]config.ValueSource {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]config.ValueSource, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
