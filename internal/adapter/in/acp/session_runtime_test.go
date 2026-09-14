package acp

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestNormalizeSessionRuntime(t *testing.T) {
	got := normalizeSessionRuntime(SessionRuntimeSettings{
		Provider:       " Protonman ",
		Model:          " qwen3.8-27b ",
		Reasoning:      " HIGH ",
		LowConcurrency: " enabled ",
	})
	if got.Provider != "Protonman" || got.Model != "qwen3.8-27b" {
		t.Fatalf("selection = %#v", got)
	}
	if got.Reasoning != "high" {
		t.Fatalf("reasoning = %q, want high", got.Reasoning)
	}
	if got.LowConcurrency != "on" {
		t.Fatalf("low concurrency = %q, want on", got.LowConcurrency)
	}
}

func TestReasoningSettingUsesAutoForDefault(t *testing.T) {
	if got := reasoningSetting(sdk.ReasoningDefault); got != "auto" {
		t.Fatalf("reasoning default = %q, want auto", got)
	}
	if got := reasoningSetting(sdk.ReasoningHigh); got != "high" {
		t.Fatalf("reasoning high = %q, want high", got)
	}
}

func TestSetSessionReasoningRejectsActivePrompt(t *testing.T) {
	server := &Server{}
	sess := &Session{id: "s1", active: true}

	err := server.setSessionReasoning(context.Background(), sess, sdk.ReasoningHigh)
	if err == nil || !strings.Contains(err.Error(), "active prompt") {
		t.Fatalf("setSessionReasoning error = %v, want active prompt rejection", err)
	}
}
