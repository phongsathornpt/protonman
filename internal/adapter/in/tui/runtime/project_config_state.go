package runtime

import "github.com/phongsathornpt/protonman/internal/adapter/out/config"

func cloneProjectConfigProvenance(in map[string]config.ValueSource) map[string]config.ValueSource {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]config.ValueSource, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func projectConfigSource(provenance map[string]config.ValueSource, field string) config.ValueSource {
	if source, ok := provenance[field]; ok {
		return source
	}
	return config.SourceDefault
}
