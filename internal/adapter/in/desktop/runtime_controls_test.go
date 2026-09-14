//go:build desktop

package desktop

import (
	"strings"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestRuntimeSummaryPreservesRuntimeFlags(t *testing.T) {
	runtime := desktopstate.RuntimeSettingsState{
		Model:          strings.Repeat("very-long-model-name-", 4),
		Reasoning:      "high",
		LowConcurrency: "on",
	}

	got := runtimeSummaryText(runtime)
	if !strings.HasSuffix(got, " · high · low") {
		t.Fatalf("runtime flags disappeared from summary: %q", got)
	}
	if len([]rune(got)) > runtimeSummaryMaxRunes {
		t.Fatalf("runtime summary exceeded %d runes: %q", runtimeSummaryMaxRunes, got)
	}
}

func TestRuntimeSummaryFallsBackToProvider(t *testing.T) {
	runtime := desktopstate.RuntimeSettingsState{Provider: "opencode", Reasoning: "auto"}
	if got := runtimeSummaryText(runtime); got != "opencode" {
		t.Fatalf("unexpected provider fallback %q", got)
	}
}
