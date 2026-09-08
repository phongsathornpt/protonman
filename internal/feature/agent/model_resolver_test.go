package agent

import (
	"context"
	"testing"

	"github.com/projectTHORN/proton/internal/engine/toolcall"
	"github.com/projectTHORN/proton/internal/engine/turn"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

type resolverTestModel struct{ id string }

func (m resolverTestModel) Provider() string { return "test" }
func (m resolverTestModel) ModelID() string  { return m.id }
func (resolverTestModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Tools: true}
}
func (resolverTestModel) Stream(context.Context, sdk.Request) (sdk.Stream, error) {
	return nil, nil
}

func TestModelResolverSnapshotsOverrides(t *testing.T) {
	original := resolverTestModel{id: "strength-a"}
	overrides := map[Profile]sdk.LanguageModel{ProfileStrength: original}
	resolver, err := NewModelResolver(overrides)
	if err != nil {
		t.Fatal(err)
	}
	overrides[ProfileStrength] = resolverTestModel{id: "mutated"}
	got := resolver.Resolve(ProfileStrength, resolverTestModel{id: "fallback"})
	if got.ModelID() != original.id {
		t.Fatalf("resolved model = %q, want immutable snapshot %q", got.ModelID(), original.id)
	}
	fallback := resolver.Resolve(ProfileAgility, resolverTestModel{id: "fallback"})
	if fallback.ModelID() != "fallback" {
		t.Fatalf("fallback model = %q, want fallback", fallback.ModelID())
	}
}

func TestModelResolverRejectsInvalidOverrides(t *testing.T) {
	if _, err := NewModelResolver(map[Profile]sdk.LanguageModel{
		ProfileUniversal: resolverTestModel{id: "main"},
	}); err == nil {
		t.Fatal("expected Universal override rejection")
	}
	if _, err := NewModelResolver(map[Profile]sdk.LanguageModel{
		ProfileAgility: nil,
	}); err == nil {
		t.Fatal("expected nil model rejection")
	}
}

func TestCoordinatorBindsModelAtAdmission(t *testing.T) {
	fallbackA := resolverTestModel{id: "universal-a"}
	fallbackB := resolverTestModel{id: "universal-b"}
	strength := resolverTestModel{id: "strength-model"}
	resolver, err := NewModelResolver(map[Profile]sdk.LanguageModel{
		ProfileStrength: strength,
	})
	if err != nil {
		t.Fatal(err)
	}
	coord := NewCoordinator(
		fallbackA,
		emptyRegistry{},
		nil,
		nil,
		WithModelResolver(resolver),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{}, nil
		}),
	)
	defer coord.Close()

	first, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "first"})
	if err != nil {
		t.Fatal(err)
	}
	coord.SetLanguageModel(fallbackB)
	second, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "second"})
	if err != nil {
		t.Fatal(err)
	}
	third, err := coord.Spawn(context.Background(), Request{Profile: ProfileStrength, Task: "third"})
	if err != nil {
		t.Fatal(err)
	}
	coord.agentsMu.RLock()
	firstModel := coord.agents[first.ID].languageModel
	secondModel := coord.agents[second.ID].languageModel
	thirdModel := coord.agents[third.ID].languageModel
	coord.agentsMu.RUnlock()

	if firstModel.ModelID() != fallbackA.id {
		t.Fatalf("first inherited model = %q, want %q", firstModel.ModelID(), fallbackA.id)
	}
	if secondModel.ModelID() != fallbackB.id {
		t.Fatalf("second inherited model = %q, want %q", secondModel.ModelID(), fallbackB.id)
	}
	if thirdModel.ModelID() != strength.id {
		t.Fatalf("strength override = %q, want %q", thirdModel.ModelID(), strength.id)
	}
}

func TestCoordinatorModelResolverUpdateAffectsFutureAdmissionsOnly(t *testing.T) {
	fallback := resolverTestModel{id: "fallback"}
	override := resolverTestModel{id: "agility-model"}
	coord := NewCoordinator(fallback, emptyRegistry{}, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{}, nil
		}),
	)
	defer coord.Close()

	first, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "first"})
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewModelResolver(map[Profile]sdk.LanguageModel{ProfileAgility: override})
	if err != nil {
		t.Fatal(err)
	}
	coord.SetModelResolver(resolver)
	second, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "second"})
	if err != nil {
		t.Fatal(err)
	}

	coord.agentsMu.RLock()
	firstModel := coord.agents[first.ID].languageModel
	secondModel := coord.agents[second.ID].languageModel
	coord.agentsMu.RUnlock()
	if firstModel.ModelID() != fallback.id || secondModel.ModelID() != override.id {
		t.Fatalf("bound models = %q, %q; want %q, %q", firstModel.ModelID(), secondModel.ModelID(), fallback.id, override.id)
	}
}

func TestCoordinatorReportsBoundModelIdentity(t *testing.T) {
	model := resolverTestModel{id: "agility-fast"}
	resolver, err := NewModelResolver(map[Profile]sdk.LanguageModel{ProfileAgility: model})
	if err != nil {
		t.Fatal(err)
	}
	coord := NewCoordinator(resolverTestModel{id: "fallback"}, emptyRegistry{}, nil, nil,
		WithModelResolver(resolver),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{}, nil
		}),
	)
	defer coord.Close()

	result, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "test" || result.Model != model.id {
		t.Fatalf("result identity = %q/%q, want test/%s", result.Provider, result.Model, model.id)
	}
	statuses := coord.List()
	if len(statuses) != 1 {
		t.Fatalf("statuses = %#v, want one retained agent", statuses)
	}
	if statuses[0].Provider != "test" || statuses[0].Model != model.id {
		t.Fatalf("status identity = %q/%q, want test/%s", statuses[0].Provider, statuses[0].Model, model.id)
	}
}
