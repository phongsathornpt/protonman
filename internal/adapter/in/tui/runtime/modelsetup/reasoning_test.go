package modelsetup

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestCompatibleReasoningFallsBackToDefault(t *testing.T) {
	md := model.RemoteModel{ID: "plain-model"}
	if got := CompatibleReasoning("custom", md, sdk.ReasoningHigh); got != sdk.ReasoningDefault {
		t.Fatalf("reasoning = %q, want default", got)
	}
}
