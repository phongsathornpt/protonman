package tui

import (
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func TestSlashReasoningUsesActiveModelProfile(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeProvider = "protonman"
	m.activeModel = "gemini-3.8-flash"

	m.executeCommand("/reasoning high")
	if got := m.reasoningEffort; got != sdk.ReasoningHigh {
		t.Fatalf("reasoningEffort = %q, want high", got)
	}

	m.executeCommand("/reasoning xhigh")
	if got := m.reasoningEffort; got != sdk.ReasoningHigh {
		t.Fatalf("unsupported xhigh changed reasoningEffort to %q", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, "does not support reasoning effort") || !strings.Contains(got, "low, medium, high") {
		t.Fatalf("transcript missing model-profile rejection: %q", got)
	}
}

func TestSlashReasoningAutoResetsSessionOverride(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "gemini-3.8-flash"
	m.reasoningEffort = sdk.ReasoningHigh

	m.executeCommand("/reasoning auto")
	if got := m.reasoningEffort; got != sdk.ReasoningDefault {
		t.Fatalf("reasoningEffort = %q, want auto/default", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, "Reasoning effort set to auto") {
		t.Fatalf("transcript = %q", got)
	}
}

func TestSlashReasoningShowsProfileLevelsAndDefault(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeProvider = "protonman"
	m.activeModel = "gemini-3.8-flash"
	m.agentProfile = "dex"

	m.executeCommand("/reasoning")
	got := plainTranscript(m)
	for _, want := range []string{
		"Reasoning override: auto",
		"Model profile: gemini-3.8-flash",
		"Supported reasoning: low, medium, high",
		"Model default: medium",
		"Agent profile preference: high",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("transcript missing %q: %q", want, got)
		}
	}
}

func TestSlashReasoningSyncsCoordinator(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "gemini-3.8-flash"
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer func() { _ = coord.Close() }()
	m.coordinator = coord

	m.executeCommand("/reasoning low")
	if got := coord.ReasoningEffort(); got != sdk.ReasoningLow {
		t.Fatalf("coordinator reasoning = %q, want low", got)
	}
	m.executeCommand("/reasoning auto")
	if got := coord.ReasoningEffort(); got != sdk.ReasoningDefault {
		t.Fatalf("coordinator reasoning = %q, want auto/default", got)
	}
}

func TestRemoteModelReasoningSummaryUsesResolvedProfile(t *testing.T) {
	md := model.RemoteModel{ID: "gemini-3.8-flash"}
	if got := remoteModelReasoningSummary("protonman", md, true); got != "reasoning low/medium/high (default medium)" {
		t.Fatalf("summary = %q", got)
	}
	if got := remoteModelReasoningSummary("custom", model.RemoteModel{ID: "future-model"}, true); got != "" {
		t.Fatalf("unknown summary = %q, want empty", got)
	}
}
