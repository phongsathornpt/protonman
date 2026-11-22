//go:build desktop || desktop_gio

package runtime_test

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/runtime"
)

// Model catalog policy (free-tier classification, curated provider fallbacks,
// partitioning) lives in internal/base/modelcatalog and is tested there. This
// package only owns the agent/provider scoping decisions that are specific to
// the runtime popover.

func TestIsProtonmanAgent(t *testing.T) {
	for _, agentID := range []string{"protonman", "ProtonMan", "proton", "", "   "} {
		if !runtime.IsProtonmanAgent(agentID) {
			t.Errorf("expected IsProtonmanAgent(%q) to be true", agentID)
		}
	}
	if runtime.IsProtonmanAgent("cline") {
		t.Error("expected IsProtonmanAgent(\"cline\") to be false")
	}
}
