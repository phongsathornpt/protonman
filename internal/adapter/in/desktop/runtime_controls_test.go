//go:build desktop

package desktop

import (
	"reflect"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
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

func TestRuntimeSummaryFallsBackToModelLabel(t *testing.T) {
	if got := runtimeSummaryText(desktopstate.RuntimeSettingsState{}); got != "Model" {
		t.Fatalf("unexpected empty runtime fallback %q", got)
	}
}

func TestSessionConfigValuesUsesAdvertisedValues(t *testing.T) {
	options := []acpclient.SessionConfigOption{{
		ID:           "model",
		CurrentValue: "qwen3.8-27b",
		Options: []acpclient.SessionConfigSelectOption{
			{Value: "qwen3.8-27b", Name: "Qwen 3.8 27B"},
			{Value: "gemini-3.8-flash", Name: "Gemini 3.8 Flash"},
		},
	}}

	got := sessionConfigValues(options, "model", "qwen3.8-27b")
	want := []string{"qwen3.8-27b", "gemini-3.8-flash"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("values = %#v, want %#v", got, want)
	}
}

func TestSessionConfigValuesPreservesCurrentValue(t *testing.T) {
	options := []acpclient.SessionConfigOption{{
		ID: "reasoning",
		Options: []acpclient.SessionConfigSelectOption{{Value: "low", Name: "Low"}},
	}}

	got := sessionConfigValues(options, "reasoning", "medium")
	want := []string{"medium", "low"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("values = %#v, want %#v", got, want)
	}
}
