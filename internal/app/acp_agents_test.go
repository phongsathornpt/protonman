package app

import (
	"context"
	"reflect"
	"testing"
)

type acpAgentsMemoryRepository struct {
	items []ACPAgentProfile
	saved []ACPAgentProfile
	err   error
}

func (r *acpAgentsMemoryRepository) Load(context.Context) ([]ACPAgentProfile, error) {
	return r.items, r.err
}

func (r *acpAgentsMemoryRepository) Save(_ context.Context, profiles []ACPAgentProfile) error {
	r.saved = cloneACPAgents(profiles)
	return r.err
}

func cloneACPAgents(profiles []ACPAgentProfile) []ACPAgentProfile {
	cloned := make([]ACPAgentProfile, len(profiles))
	for index, profile := range profiles {
		cloned[index] = cloneACPAgentProfile(profile)
	}
	return cloned
}

func TestACPAgentsLoadNormalizesAndMigratesProfiles(t *testing.T) {
	repository := &acpAgentsMemoryRepository{items: []ACPAgentProfile{
		{ID: " Reviewer ", DisplayName: " Review agent ", Command: " reviewer ", Args: []string{" --stdio ", ""}, Env: []string{"TOKEN=secret", "TOKEN", "REGION"}},
		{ID: "protonman", Command: "protonman", Args: []string{"--acp"}},
	}}
	agents := NewACPAgents(repository)

	profiles, err := agents.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []ACPAgentProfile{
		{ID: "protonman", DisplayName: "protonman", Command: "protonman", Args: []string{"--acp"}},
		{ID: "reviewer", DisplayName: "Review agent", Command: "reviewer", Args: []string{"--stdio"}, Env: []string{"TOKEN", "REGION"}},
	}
	if !reflect.DeepEqual(profiles, want) {
		t.Fatalf("profiles = %#v, want %#v", profiles, want)
	}
	if !reflect.DeepEqual(repository.saved, want) {
		t.Fatalf("migrated profiles = %#v, want %#v", repository.saved, want)
	}
}

func TestACPAgentsResolvePrefersEnvironmentOverride(t *testing.T) {
	repository := &acpAgentsMemoryRepository{items: []ACPAgentProfile{{ID: "saved", Command: "saved"}}}
	agents := NewACPAgents(repository)
	profiles, err := agents.Resolve(context.Background(), []ACPAgentProfile{{ID: "protonman", Command: "protonman"}}, `[{
		"id":"custom",
		"displayName":"Custom ACP",
		"command":"custom-acp",
		"args":["--stdio"],
		"env":["TOKEN=secret"]
	}]`)
	if err != nil {
		t.Fatal(err)
	}
	want := []ACPAgentProfile{{ID: "custom", DisplayName: "Custom ACP", Command: "custom-acp", Args: []string{"--stdio"}, Env: []string{"TOKEN=secret"}}}
	if !reflect.DeepEqual(profiles, want) {
		t.Fatalf("profiles = %#v, want %#v", profiles, want)
	}
}

func TestACPAgentsResolveUsesDefaultsWhenRepositoryIsEmpty(t *testing.T) {
	agents := NewACPAgents(&acpAgentsMemoryRepository{})
	defaults := []ACPAgentProfile{{ID: "protonman", Command: "protonman", Args: []string{"--acp"}}}
	profiles, err := agents.Resolve(context.Background(), defaults, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ID != "protonman" || len(profiles[0].Args) != 1 || profiles[0].Args[0] != "--acp" {
		t.Fatalf("profiles = %#v", profiles)
	}
}

func TestACPAgentsSaveCopiesAndValidatesProfiles(t *testing.T) {
	repository := &acpAgentsMemoryRepository{}
	agents := NewACPAgents(repository)
	profiles := []ACPAgentProfile{{ID: "protonman", Command: "protonman", Args: []string{"--acp"}, Env: []string{"TOKEN"}}}
	if err := agents.Save(context.Background(), profiles); err != nil {
		t.Fatal(err)
	}
	profiles[0].Args[0] = "changed"
	if repository.saved[0].Args[0] != "--acp" {
		t.Fatalf("saved args alias input: %#v", repository.saved)
	}
	if err := agents.Save(context.Background(), []ACPAgentProfile{{ID: "Bad ID", Command: "agent"}}); err == nil {
		t.Fatal("expected invalid ACP agent id to fail")
	}
}
