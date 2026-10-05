package slashview

import (
	"strings"
	"testing"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

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
	if !FuzzyContains("résumé-modèle", "rmd") {
		t.Fatal("expected unicode rune fuzzy match")
	}
	if !FuzzyContains("日本語モデル", "日モ") {
		t.Fatal("expected cjk unicode rune fuzzy match")
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
	}, tuistyle.UnicodeIcons)
	if len(matches) != 1 {
		t.Fatalf("matches = %#v", matches)
	}
	if matches[0].Name != "pdf-processing" || matches[0].PrefixTag != strings.TrimSpace(tuistyle.UnicodeTodoActive) || matches[0].Scope != "user" {
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
	matches := Matches(context, Catalog(), nil, tuistyle.UnicodeIcons)
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

func TestIsCommandLineDistinguishesFilePathsAndCommands(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid slash commands (known and unknown)
		{"/help", true},
		{":help", true},
		{"/quit", true},
		{":quit", true},
		{"/skills active", true},
		{"/model gpt-4o", true},
		{"/definitely-missing", true},
		{"/unknown-cmd arg1 arg2", true},

		// File paths (Unix absolute paths, extensions, slashes)
		{"/Users/alice/Desktop/screenshot.png", false},
		{"/home/user/pictures/photo.jpg", false},
		{"/image.png", false},
		{"/foo/bar", false},
		{"/var/log/syslog", false},
		{"/path/to/diagram.webp what is this?", false},

		// Non-command prefixes
		{"// comment", false},
		{":: comment", false},
		{"/", false},
		{":", false},
		{"/ ", false},
		{"/:", false},
		{"plain text", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsCommandLine(tt.input)
			if got != tt.want {
				t.Errorf("IsCommandLine(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCommandMatchesIncludeActiveSkills(t *testing.T) {
	context, ok := ParseContext("/gol")
	if !ok {
		t.Fatal("ParseContext returned false")
	}
	catalog := []Command{
		{Name: "goal", Description: "manage active goal"},
		{Name: "help", Description: "list commands"},
	}
	skills := []Skill{
		{Name: "golang-code-review", Description: "review go code", Active: true},
		{Name: "inactive-skill", Description: "golang test", Active: false},
	}
	matches := Matches(context, catalog, skills, tuistyle.UnicodeIcons)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (/goal and the golang-code-review skill), got %d", len(matches))
	}
	if matches[0].Name != "goal" {
		t.Fatalf("matches[0].Name = %q, want goal", matches[0].Name)
	}
	if matches[1].Name != "golang-code-review" {
		t.Fatalf("matches[1].Name = %q, want golang-code-review", matches[1].Name)
	}
	if matches[1].PrefixTag != strings.TrimSpace(tuistyle.UnicodeIcons.Skill) {
		t.Fatalf("matches[1].PrefixTag = %q, want the skill icon", matches[1].PrefixTag)
	}
	if matches[1].Argument != ArgumentRest {
		t.Fatalf("matches[1].Argument = %v, want ArgumentRest", matches[1].Argument)
	}
	if !matches[1].EchoUser {
		t.Fatal("expected matches[1].EchoUser = true")
	}
}
