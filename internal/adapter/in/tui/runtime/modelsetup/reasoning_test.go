package modelsetup

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

func TestCompatibleReasoningFallsBackToDefault(t *testing.T) {
	md := model.RemoteModel{ID: "plain-model"}
	if got := CompatibleReasoning("custom", md, domain.ReasoningHigh); got != domain.ReasoningDefault {
		t.Fatalf("reasoning = %q, want default", got)
	}
}
