package slashview

import "testing"

func TestCatalogContainsCanonicalCommandsOnly(t *testing.T) {
	catalog := Catalog()
	want := []string{"help", "permission", "low", "model", "provider", "skills", "agents", "goal", "todo", "clear", "resume", "call", "quit"}
	if len(catalog) != len(want) {
		t.Fatalf("catalog size = %d, want %d: %#v", len(catalog), len(want), catalog)
	}
	for i, name := range want {
		if catalog[i].Name != name {
			t.Fatalf("catalog[%d] = %q, want %q", i, catalog[i].Name, name)
		}
	}
}

func TestParseContext(t *testing.T) {
	tests := []struct {
		value string
		ok    bool
		kind  ContextKind
		lead  string
		query string
	}{
		{value: "/he", ok: true, kind: ContextCommand, lead: "/", query: "he"},
		{value: ":model", ok: true, kind: ContextCommand, lead: ":", query: "model"},
		{value: "/skills pd", ok: true, kind: ContextSkill, lead: "/skills ", query: "pd"},
		{value: "/skills toggle pdf", ok: true, kind: ContextSkill, lead: "/skills toggle ", query: "pdf"},
		{value: "/skills toggle", ok: false},
		{value: "/low ", ok: true, kind: ContextLowConcurrency, lead: "/low ", query: ""},
		{value: "/low a", ok: true, kind: ContextLowConcurrency, lead: "/low ", query: "a"},
		{value: "/skill pd", ok: false},
		{value: "/model free", ok: false},
		{value: "plain", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, ok := ParseContext(tt.value)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if got.Kind != tt.kind || got.Lead != tt.lead || got.Query != tt.query {
				t.Fatalf("context = %#v", got)
			}
		})
	}
}

func TestCanonicalAndFuzzyMatching(t *testing.T) {
	if got := CanonicalName("MODEL"); got != "model" {
		t.Fatalf("CanonicalName = %q", got)
	}
	if !FuzzyContains("provider", "pvd") {
		t.Fatal("expected subsequence fuzzy match")
	}
	if FuzzyContains("provider", "pxd") {
		t.Fatal("unexpected fuzzy match")
	}
}

func TestSkillMatchesCarryPresentationMetadata(t *testing.T) {
	context, ok := ParseContext("/skills pdf")
	if !ok {
		t.Fatal("ParseContext returned false")
	}
	matches := Matches(context, nil, []Skill{
		{Name: "pdf-processing", Description: "work with pdf files", Scope: "user", Active: true},
		{Name: "slides", Description: "presentations", Scope: "project"},
	})
	if len(matches) != 1 {
		t.Fatalf("matches = %#v", matches)
	}
	if matches[0].Name != "pdf-processing" || matches[0].PrefixTag != "[x]" || matches[0].Scope != "user" {
		t.Fatalf("match = %#v", matches[0])
	}
}

func TestParseCommandPreservesFullTrailingArgument(t *testing.T) {
	parsed := ParseCommand("/goal implement model-aware compaction safely")
	if parsed.Name != "goal" || parsed.Argument != "implement" || parsed.Rest != "implement model-aware compaction safely" {
		t.Fatalf("parsed command = %#v", parsed)
	}
}

func TestCatalogDeclaresArgumentModes(t *testing.T) {
	goal, ok := LookupCommand("GOAL")
	if !ok || goal.Argument != ArgumentRest {
		t.Fatalf("goal spec = %#v, ok=%v", goal, ok)
	}
	clear, ok := LookupCommand("clear")
	if !ok || clear.Argument != ArgumentNone {
		t.Fatalf("clear spec = %#v, ok=%v", clear, ok)
	}
}

func TestLowConcurrencyArgumentMatches(t *testing.T) {
	context, ok := ParseContext("/low a")
	if !ok {
		t.Fatal("ParseContext returned false")
	}
	matches := Matches(context, Catalog(), nil)
	if len(matches) != 1 || matches[0].Name != "auto" {
		t.Fatalf("matches = %#v, want auto", matches)
	}
}

func TestCommandPresentation(t *testing.T) {
	command := Command{Name: "pdf-processing", Description: "work with pdf files", PrefixTag: "[x]", Scope: "user"}
	if got := command.FilterValue(); got != "pdf-processing work with pdf files" {
		t.Fatalf("FilterValue = %q", got)
	}
	if got := command.Title(); got != "[x] pdf-processing" {
		t.Fatalf("Title = %q", got)
	}
	if got := command.DisplayDescription(); got != "work with pdf files · user" {
		t.Fatalf("DisplayDescription = %q", got)
	}
}
