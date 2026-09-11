package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestPrimaryConversationProfileDoesNotBecomeSubagentRole(t *testing.T) {
	prompt, _, err := primaryConversationPolicy(ConversationSpec{AgentProfile: string(agent.ProfileStrength)})
	if err != nil {
		t.Fatal(err)
	}
	if prompt.Profile != string(agent.ProfileStrength) {
		t.Fatalf("profile = %q", prompt.Profile)
	}
	if prompt.Role != "" {
		t.Fatalf("primary conversation role = %q, want empty", prompt.Role)
	}
}

func TestPrimaryConversationRejectsUnknownProfile(t *testing.T) {
	if _, _, err := primaryConversationPolicy(ConversationSpec{AgentProfile: "unknown"}); err == nil {
		t.Fatal("expected invalid profile error")
	}
}

func TestPrimaryConversationPolicyCarriesActiveGoal(t *testing.T) {
	got, _, err := primaryConversationPolicy(ConversationSpec{ActiveGoal: "  preserve this goal  "})
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveGoal != "preserve this goal" {
		t.Fatalf("active goal = %q", got.ActiveGoal)
	}
}

func TestSynthesisBatchMessagesUseStructuredResultPayload(t *testing.T) {
	batch := agent.SynthesisBatch{Results: []agent.SynthesisResult{{
		Result: agent.Result{
			AgentID: "agility-1", Profile: agent.ProfileAgility,
			Conclusion: "found reconnect race",
			Findings:   []agent.Finding{{Claim: "listener registers twice", Confidence: "high", Evidence: []agent.EvidenceRef{{Tool: "read", Target: "session.go"}}}},
			Evidence:   []agent.EvidenceRef{{Tool: "read", Target: "session.go"}}, ChangedTargets: []string{"session.go"},
		},
	}}}
	messages, err := synthesisBatchMessages(batch)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages=%d, want 1", len(messages))
	}
	for _, want := range []string{`"status":"completed"`, `"conclusion":"found reconnect race"`, `"evidence"`, `"changed_targets"`} {
		if !strings.Contains(messages[0].Content, want) {
			t.Fatalf("runtime context missing %q: %s", want, messages[0].Content)
		}
	}
	if strings.Contains(messages[0].Content, `"summary"`) {
		t.Fatalf("runtime context retained legacy summary field: %s", messages[0].Content)
	}
}

func TestSynthesisBatchMessagesAcceptLegacySummaryOnly(t *testing.T) {
	batch := agent.SynthesisBatch{Results: []agent.SynthesisResult{{Result: agent.Result{
		AgentID: "agility-legacy", Profile: agent.ProfileAgility, Summary: "legacy conclusion",
	}}}}
	messages, err := synthesisBatchMessages(batch)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || !strings.Contains(messages[0].Content, `"conclusion":"legacy conclusion"`) {
		t.Fatalf("legacy synthesis context=%#v", messages)
	}
}

func TestSynthesisBatchMessagesExposeBoundedFailureBlocker(t *testing.T) {
	batch := agent.SynthesisBatch{Results: []agent.SynthesisResult{{
		Result: agent.Result{AgentID: "agility-1", Profile: agent.ProfileAgility, Err: errors.New(strings.Repeat("x", runtimeSubagentBlockerBytes*2))},
	}}}
	messages, err := synthesisBatchMessages(batch)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(messages[0].Content, `"status":"failed"`) || !strings.Contains(messages[0].Content, `"blockers"`) {
		t.Fatalf("failure context missing status/blocker: %s", messages[0].Content)
	}
	if len(messages[0].Content) > runtimeSubagentBlockerBytes+1024 {
		t.Fatalf("failure context is unexpectedly large: %d bytes", len(messages[0].Content))
	}
}

type blockingRuntimeAgentRunner struct{}

func (blockingRuntimeAgentRunner) Run(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
	<-ctx.Done()
	return turn.Result{}, ctx.Err()
}

func TestSubagentRuntimeContextOptionalChildIsNonBlockingAndFinalized(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return blockingRuntimeAgentRunner{}, nil
		}),
	)
	defer coord.Close()
	ref := agent.TurnRef{SessionID: "session-a", TurnID: "turn-a"}
	ctx := agent.WithTurnRef(context.Background(), ref)
	handle, err := coord.Spawn(ctx, agent.Request{
		SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: agent.ProfileAgility,
		Task: "speculative", Optional: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := newSubagentRuntimeContextProvider(NewAgents(coord))
	if !provider.Active(ctx) {
		t.Fatal("optional child should keep runtime context active for safe buffering")
	}
	if provider.Pending(ctx) {
		t.Fatal("optional child must not hold the completion barrier")
	}
	provider.Finalize(ctx)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, ok := coord.Get(handle.ID)
		if ok && status.State.Terminal() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	status, _ := coord.Get(handle.ID)
	t.Fatalf("optional child remained live after finalization: %+v", status)
}

func TestSubagentRuntimeContextMarksDeliveredResultConsumed(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return immediateRuntimeAgentRunner{}, nil
		}),
	)
	defer coord.Close()
	ref := agent.TurnRef{SessionID: "session-consumed", TurnID: "turn-consumed"}
	ctx := agent.WithTurnRef(context.Background(), ref)
	handle, err := coord.Spawn(ctx, agent.Request{
		SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: agent.ProfileAgility, Task: "inspect",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coord.Wait(ctx, handle.ID, time.Second); err != nil {
		t.Fatal(err)
	}
	events, unsubscribe := coord.Subscribe(4)
	defer unsubscribe()
	provider := newSubagentRuntimeContextProvider(NewAgents(coord))
	messages, err := provider.Drain(ctx)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages=%#v err=%v", messages, err)
	}
	select {
	case event := <-events:
		if event.Kind != agent.EventAgentResultConsumed || event.AgentID != handle.ID || event.ResultVersion == 0 {
			t.Fatalf("consumed event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing result-consumed event")
	}
}

type immediateRuntimeAgentRunner struct{}

func (immediateRuntimeAgentRunner) Run(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
	return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
}

func TestSubagentRuntimeContextFinalizeCancelsRequiredChild(t *testing.T) {
	coord := agent.NewCoordinator(nil, nil, nil, nil,
		agent.WithRunnerFactory(func(agent.Profile, *toolcall.Service) (turn.Runner, error) {
			return blockingRuntimeAgentRunner{}, nil
		}),
	)
	defer coord.Close()
	ref := agent.TurnRef{SessionID: "session-required", TurnID: "turn-required"}
	ctx := agent.WithTurnRef(context.Background(), ref)
	handle, err := coord.Spawn(ctx, agent.Request{
		SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: agent.ProfileStrength, Task: "required child",
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := newSubagentRuntimeContextProvider(NewAgents(coord))
	provider.Finalize(ctx)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, ok := coord.Get(handle.ID)
		if ok && status.State.Terminal() {
			if status.State != agent.StateCanceled {
				t.Fatalf("required child state=%s, want canceled", status.State)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	status, _ := coord.Get(handle.ID)
	t.Fatalf("required child remained live after parent finalization: %+v", status)
}
