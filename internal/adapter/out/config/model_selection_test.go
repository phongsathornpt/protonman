package config

import "testing"

func TestReconcileModelSelectionPreservesValidPair(t *testing.T) {
	current := ModelConfig{Default: "saved-model", Provider: "protonman"}
	got, changed := ReconcileModelSelection(current, map[string]ProviderConfig{"protonman": {Name: "protonman"}})
	if changed || got != current {
		t.Fatalf("got %+v changed=%v, want unchanged %+v", got, changed, current)
	}
}

func TestReconcileModelSelectionDropsModelForMissingProvider(t *testing.T) {
	current := ModelConfig{Default: "stale-model", Provider: "beta"}
	providers := map[string]ProviderConfig{"protonman": {Name: "protonman"}, "opencode": {Name: "opencode"}}
	got, changed := ReconcileModelSelection(current, providers)
	if !changed {
		t.Fatal("expected stale selection to be reconciled")
	}
	if got.Provider != "" || got.Default != "" {
		t.Fatalf("got %+v, want unresolved selection cleared for application fallback", got)
	}
}
