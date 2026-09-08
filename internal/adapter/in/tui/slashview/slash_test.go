package slashview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCatalogKeepsCanonicalSkillsCommand(t *testing.T) {
	catalog := Catalog("strength|agility|intelligence")
	count := 0
	for _, command := range catalog {
		if command.Name != "skills" {
			continue
		}
		count++
		if len(command.Aliases) != 1 || command.Aliases[0] != "skill" {
			t.Fatalf("skills aliases = %#v", command.Aliases)
		}
	}
	if count != 1 {
		t.Fatalf("skills command count = %d, want 1", count)
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
		{value: ":models", ok: true, kind: ContextCommand, lead: ":", query: "models"},
		{value: "/skill pd", ok: true, kind: ContextSkill, lead: "/skill ", query: "pd"},
		{value: "/skills toggle pdf", ok: true, kind: ContextSkill, lead: "/skills toggle ", query: "pdf"},
		{value: "/skills toggle", ok: false},
		{value: "/model free", ok: false},
		{value: "plain", ok: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, ok := ParseContext(test.value)
			if ok != test.ok {
				t.Fatalf("ok = %v, want %v", ok, test.ok)
			}
			if !ok {
				return
			}
			if got.Kind != test.kind || got.Lead != test.lead || got.Query != test.query {
				t.Fatalf("context = %#v", got)
			}
		})
	}
}

func TestCanonicalAndFuzzyMatching(t *testing.T) {
	catalog := Catalog("strength|agility|intelligence")
	if got := CanonicalName(catalog, "MODELS"); got != "model" {
		t.Fatalf("CanonicalName(models) = %q", got)
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

func TestRenderShowsSelectedWindow(t *testing.T) {
	matches := []Command{
		{Name: "one", Description: "first"},
		{Name: "two", Description: "second"},
		{Name: "three", Description: "third"},
	}
	plain := ansi.Strip(Render(matches, 2, 80, 2, ContextCommand))
	if strings.Contains(plain, "/one") || !strings.Contains(plain, "/two") || !strings.Contains(plain, "/three") {
		t.Fatalf("unexpected render window: %q", plain)
	}
	if !strings.Contains(plain, "item 3 of 3") {
		t.Fatalf("missing position hint: %q", plain)
	}
}
