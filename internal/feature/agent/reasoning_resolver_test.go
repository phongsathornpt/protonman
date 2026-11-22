package agent

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

func TestReasoningResolverSnapshotsOverrides(t *testing.T) {
	overrides := map[Profile]domain.ReasoningEffort{ProfileIntelligence: domain.ReasoningHigh}
	resolver, err := NewReasoningResolver(overrides)
	if err != nil {
		t.Fatal(err)
	}
	overrides[ProfileIntelligence] = domain.ReasoningLow
	if got := resolver.Resolve(ProfileIntelligence, domain.ReasoningMedium); got != domain.ReasoningHigh {
		t.Fatalf("resolved reasoning = %q, want high", got)
	}
	if got := resolver.Resolve(ProfileAgility, domain.ReasoningLow); got != domain.ReasoningLow {
		t.Fatalf("fallback reasoning = %q, want low", got)
	}
}

func TestReasoningResolverTreatsAutoAsFallback(t *testing.T) {
	resolver, err := NewReasoningResolver(map[Profile]domain.ReasoningEffort{
		ProfileAgility: domain.ReasoningDefault,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resolver.Resolve(ProfileAgility, domain.ReasoningMedium); got != domain.ReasoningMedium {
		t.Fatalf("auto reasoning = %q, want global fallback medium", got)
	}
}

func TestReasoningResolverRejectsInvalidOverrides(t *testing.T) {
	if _, err := NewReasoningResolver(map[Profile]domain.ReasoningEffort{
		ProfileUniversal: domain.ReasoningHigh,
	}); err == nil {
		t.Fatal("expected Universal reasoning override rejection")
	}
	if _, err := NewReasoningResolver(map[Profile]domain.ReasoningEffort{
		ProfileStrength: domain.ReasoningEffort("turbo"),
	}); err == nil {
		t.Fatal("expected invalid reasoning rejection")
	}
}

func TestCoordinatorBindsReasoningAtAdmission(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithReasoningEffort(domain.ReasoningLow),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{}, nil
		}),
	)
	defer coord.Close()
	first, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "first"})
	if err != nil {
		t.Fatal(err)
	}
	coord.SetReasoningEffort(domain.ReasoningMedium)
	resolver, err := NewReasoningResolver(map[Profile]domain.ReasoningEffort{
		ProfileIntelligence: domain.ReasoningHigh,
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
	if firstEffort != domain.ReasoningLow || secondEffort != domain.ReasoningMedium || thirdEffort != domain.ReasoningHigh {
		t.Fatalf("bound reasoning = %q, %q, %q; want low, medium, high", firstEffort, secondEffort, thirdEffort)
	}
}
