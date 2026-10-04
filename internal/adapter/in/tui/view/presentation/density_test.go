package presentation

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestMinimalPolicyKeepsMaterialToolDetail(t *testing.T) {
	policy := MinimalPolicy()
	if got := policy.ToolDetail(tool.KindRead, false, false); got != DetailSummary {
		t.Fatalf("read detail = %v, want summary", got)
	}
	if got := policy.ToolDetail(tool.KindGit, false, false); got != DetailMaterial {
		t.Fatalf("git detail = %v, want material", got)
	}
	if got := policy.ToolDetail(tool.KindRead, true, false); got != DetailDiagnostic {
		t.Fatalf("denied detail = %v, want diagnostic", got)
	}
}
