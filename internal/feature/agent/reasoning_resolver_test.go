package agent

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestReasoningResolverSnapshotsOverrides(t *testing.T) {
	overrides := map[Profile]sdk.ReasoningEffort{ProfileIntelligence: sdk.ReasoningHigh}
	resolver, err := NewReasoningResolver(overrides)
	if err != nil {
		t.Fatal(err)
	}
	overrides[ProfileIntelligence] = sdk.ReasoningLow
	if got := resolver.Resolve(ProfileIntelligence, sdk.ReasoningMedium); got != sdk.ReasoningHigh {
		t.Fatalf("resolved reasoning = %q, want high", got)
	}
	if got := resolver.Resolve(ProfileAgility, sdk.ReasoningLow); got != sdk.ReasoningLow {
		t.Fatalf("fallback reasoning = %q, want low", got)
	}
}

func TestReasoningResolverTreatsAutoAsFallback(t *testing.T) {
	resolver, err := NewReasoningResolver(map[Profile]sdk.ReasoningEffort{
		ProfileAgility: sdk.ReasoningDefault,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resolver.Resolve(ProfileAgility, sdk.ReasoningMedium); got != sdk.ReasoningMedium {
		t.Fatalf("auto reasoning = %q, want global fallback medium", got)
	}
}

func TestReasoningResolverRejectsInvalidOverrides(t *testing.T) {
	if _, err := NewReasoningResolver(map[Profile]sdk.ReasoningEffort{
		ProfileUniversal: sdk.ReasoningHigh,
	}); err == nil {
		t.Fatal("expected Universal reasoning override rejection")
	}
	if _, err := NewReasoningResolver(map[Profile]sdk.ReasoningEffort{
		ProfileStrength: sdk.ReasoningEffort("turbo"),
	}); err == nil {
		t.Fatal("expected invalid reasoning rejection")
	}
}

func TestCoordinatorBindsReasoningAtAdmission(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithReasoningEffort(sdk.ReasoningLow),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{}, nil
		}),
	)
	defer coord.Close()
	first, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "first"})
	if err != nil {
		t.Fatal(err)
	}
	coord.SetReasoningEffort(sdk.ReasoningMedium)
	resolver, err := NewReasoningResolver(map[Profile]sdk.ReasoningEffort{
		ProfileIntelligence: sdk.ReasoningHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	coord.SetReasoningResolver(resolver)
	second, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "second"})
	if err != nil {
		t.Fatal(err)
	}
	third, err := coord.Spawn(context.Background(), Request{Profile: ProfileIntelligence, Task: "third"})
	if err != nil {
		t.Fatal(err)
	}

	coord.agentsMu.RLock()
	firstEffort := coord.agents[first.ID].reasoningEffort
	secondEffort := coord.agents[second.ID].reasoningEffort
	thirdEffort := coord.agents[third.ID].reasoningEffort
	coord.agentsMu.RUnlock()
	if firstEffort != sdk.ReasoningLow || secondEffort != sdk.ReasoningMedium || thirdEffort != sdk.ReasoningHigh {
		t.Fatalf("bound reasoning = %q, %q, %q; want low, medium, high", firstEffort, secondEffort, thirdEffort)
	}
}
