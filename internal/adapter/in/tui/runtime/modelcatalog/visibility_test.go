package modelcatalog

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func TestVisibleForAccessDoesNotFallbackToPaid(t *testing.T) {
	models := []model.RemoteModel{{ID: "claude-sonnet-5"}}
	visible := VisibleForAccess("opencode", model.DefaultOpenCodeEndpoint, "", models)
	if len(visible) != 0 {
		t.Fatalf("visible models = %+v, want none when no free models are available", visible)
	}
}
