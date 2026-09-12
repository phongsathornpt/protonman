package provider

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func TestBuildSelectionEntriesOrdersConfiguredBeforePresets(t *testing.T) {
	providers := map[string]config.ProviderConfig{
		"zeta":  {Name: "Zeta", BaseURL: "https://z.example"},
		"alpha": {Name: "Alpha", BaseURL: "https://a.example"},
	}
	entries := BuildSelectionEntries(providers, "zeta")
	if len(entries) < 3 {
		t.Fatalf("entries = %d, want configured, presets, and custom", len(entries))
	}
	if entries[0].Name != "alpha" || entries[1].Name != "zeta" {
		t.Fatalf("configured order = %q, %q", entries[0].Name, entries[1].Name)
	}
	if !entries[1].IsActive {
		t.Fatal("active configured provider not marked active")
	}
}
func TestBuildSelectionEntriesMarksOpenCodeFree(t *testing.T) {
	providers := map[string]config.ProviderConfig{
		model.DefaultOpenCodeName: {Name: model.DefaultOpenCodeName, BaseURL: model.DefaultOpenCodeEndpoint},
	}
	entries := BuildSelectionEntries(providers, model.DefaultOpenCodeName)
	if len(entries) == 0 || !entries[0].IsFree {
		t.Fatalf("OpenCode entry = %+v, want free", entries)
	}
}

func TestActiveSelectionIndexFallsBackToFirst(t *testing.T) {
	entries := []SelectionEntry{{Name: "a"}, {Name: "b", IsActive: true}}
	if got := ActiveSelectionIndex(entries); got != 1 {
		t.Fatalf("active index = %d, want 1", got)
	}
	if got := ActiveSelectionIndex([]SelectionEntry{{Name: "a"}}); got != 0 {
		t.Fatalf("fallback index = %d, want 0", got)
	}
}
