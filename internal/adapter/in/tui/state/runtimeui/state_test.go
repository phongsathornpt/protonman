package runtimeui

import (
	"strings"
	"testing"
	"time"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestProjectRuntimeStatePriority(t *testing.T) {
	now := time.Date(2026, 9, 13, 1, 0, 0, 0, time.UTC)
	started := now.Add(-5 * time.Second)
	retry := sdk.RetryEvent{Attempt: 2, MaxRetries: 4, Reason: "idle_event_timeout", RetryAt: now.Add(3 * time.Second)}

	tests := []struct {
		name     string
		input    Input
		phase    Phase
		activity string
		meta     []string
	}{
		{name: "idle", input: Input{}, phase: PhaseIdle},
		{name: "working", input: Input{Busy: true, FallbackActivity: "roaming", ToolCalls: 2, StartedAt: started, Now: now}, phase: PhaseWorking, activity: "roaming", meta: []string{"2 tools", "5s"}},
		{name: "streaming", input: Input{Busy: true, Streaming: true, FallbackActivity: "roaming", StartedAt: started, Now: now}, phase: PhaseStreaming, activity: "streaming response", meta: []string{"5s"}},
		{name: "tool", input: Input{Busy: true, Streaming: true, RunningTool: "Read chrome.go", ToolCalls: 3, StartedAt: started, Now: now}, phase: PhaseToolRunning, activity: "Read chrome.go", meta: []string{"3 tools", "5s"}},
		{name: "explicit tool activity", input: Input{Busy: true, Streaming: true, RunningTool: "Read chrome.go", ExplicitActivity: "running read", ToolCalls: 3, StartedAt: started, Now: now}, phase: PhaseToolRunning, activity: "running read", meta: []string{"3 tools", "5s"}},
		{name: "agents", input: Input{Busy: true, RunningTool: "Read chrome.go", ActiveAgents: 2, AgentActivity: "pushing", StartedAt: started, Now: now}, phase: PhaseDelegating, activity: "pushing", meta: []string{"2 agents", "5s"}},
		{name: "retry", input: Input{Busy: true, ActiveAgents: 2, Retry: retry, StartedAt: started, Now: now}, phase: PhaseRetryWaiting, activity: "retrying in 3s", meta: []string{"retry 2/4", "stream stalled", "5s"}},
		{name: "cancel", input: Input{Busy: true, Canceling: true, Retry: retry, ActiveAgents: 2, AgentActivity: "retreating", StartedAt: started, Now: now}, phase: PhaseCanceling, activity: "retreating", meta: []string{"2 agents", "5s"}},
		{name: "permission", input: Input{Busy: true, PermissionPending: true, Canceling: true, Retry: retry, StartedAt: started, Now: now}, phase: PhaseWaitingForInput, activity: "action required", meta: []string{"permission", "5s"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Project(tt.input)
			if got.Phase != tt.phase {
				t.Fatalf("phase = %v, want %v", got.Phase, tt.phase)
			}
			if got.Activity != tt.activity {
				t.Fatalf("activity = %q, want %q", got.Activity, tt.activity)
			}
			if strings.Join(got.Meta, "|") != strings.Join(tt.meta, "|") {
				t.Fatalf("meta = %#v, want %#v", got.Meta, tt.meta)
			}
		})
	}
}

func TestProjectRetryCooldownCopy(t *testing.T) {
	now := time.Date(2026, 9, 13, 1, 0, 0, 0, time.UTC)
	state := Project(Input{
		Busy: true,
		Retry: sdk.RetryEvent{
			Attempt:    1,
			MaxRetries: 3,
			Reason:     "overloaded",
			RetryAt:    now.Add(1500 * time.Millisecond),
			Phase:      sdk.RetryPhaseCooldown,
		},
		Now: now,
	})
	if state.Phase != PhaseRetryWaiting || state.Activity != "cooling down 2s" {
		t.Fatalf("state = %#v", state)
	}
	if got := state.MetaText(); got != "retry 1/3 · provider busy" {
		t.Fatalf("meta = %q", got)
	}
}

func TestPhaseStringStable(t *testing.T) {
	want := map[Phase]string{
		PhaseIdle:            "idle",
		PhaseWorking:         "working",
		PhaseStreaming:       "streaming",
		PhaseToolRunning:     "tool",
		PhaseDelegating:      "delegating",
		PhaseRetryWaiting:    "retry",
		PhaseWaitingForInput: "waiting-input",
		PhaseCanceling:       "canceling",
	}
	for phase, label := range want {
		if got := phase.String(); got != label {
			t.Fatalf("%v string = %q, want %q", phase, got, label)
		}
	}
}
