package projectconfig

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
)

func TestCloneProvenanceDoesNotShareMap(t *testing.T) {
	in := map[string]config.ValueSource{"model.default": config.SourceUser}
	out := CloneProvenance(in)
	out["model.default"] = config.SourceProject
	if in["model.default"] != config.SourceUser {
		t.Fatal("clone mutated source map")
	}
}
