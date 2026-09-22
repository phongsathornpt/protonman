package cmdpolicy

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/slashview"
)

func TestClassifyCommands(t *testing.T) {
	tests := []struct {
		line     string
		wantKind Kind
		wantName string
		wantArg  string
	}{
		{"/help", KindHelp, "help", ""},
		{"/model gpt-4o", KindModel, "model", "gpt-4o"},
		{"/provider add", KindProvider, "provider", "add"},
		{"/low on", KindLow, "low", "on"},
		{"/clear", KindClear, "clear", ""},
		{"/resume abc", KindResume, "resume", "abc"},
		{"/todo show", KindTodo, "todo", "show"},
		{"/goal fix bug", KindGoal, "goal", "fix"},
		{"/quit", KindQuit, "quit", ""},
		{"/unknown-cmd", KindUnknown, "unknown-cmd", ""},
	}

	for _, tc := range tests {
		cmd := Classify(tc.line)
		if cmd.Kind != tc.wantKind {
			t.Errorf("Classify(%q).Kind = %v; want %v", tc.line, cmd.Kind, tc.wantKind)
		}
		if cmd.Name != tc.wantName {
			t.Errorf("Classify(%q).Name = %q; want %q", tc.line, cmd.Name, tc.wantName)
		}
		if cmd.Argument != tc.wantArg {
			t.Errorf("Classify(%q).Argument = %q; want %q", tc.line, cmd.Argument, tc.wantArg)
		}
	}
}

func TestCommandIsKnown(t *testing.T) {
	if !Classify("/help").IsKnown() {
		t.Error("expected /help to be known")
	}
	if Classify("/not-a-command").IsKnown() {
		t.Error("expected /not-a-command to be unknown")
	}
}

// TestKindTableMatchesCatalog enforces exact set equality between the
// cmdpolicy kind table and the slashview completion catalog, in both
// directions, so a command added to only one registry fails loudly.
func TestKindTableMatchesCatalog(t *testing.T) {
	catalog := make(map[string]bool)
	for _, command := range slashview.Catalog() {
		catalog[command.Name] = true
	}
	for name := range kindsByName {
		if !catalog[name] {
			t.Errorf("kind table has %q but slashview.Catalog does not; add it to the catalog or drop the stale entry", name)
		}
	}
	for name := range catalog {
		if _, ok := kindsByName[name]; !ok {
			t.Errorf("catalog command %q has no cmdpolicy kind; add it to kindsByName", name)
		}
	}
}

// TestEveryCatalogCommandClassifies walks the real classification pipeline
// (ParseCommand -> kindsByName) for every catalog entry.
func TestEveryCatalogCommandClassifies(t *testing.T) {
	for _, command := range slashview.Catalog() {
		got := Classify("/" + command.Name)
		if got.Kind == KindUnknown {
			t.Errorf("Classify(%q).Kind is KindUnknown; every catalog command needs a kind", command.Name)
		}
		if got.Name != command.Name {
			t.Errorf("Classify(%q).Name = %q, want %q", command.Name, got.Name, command.Name)
		}
	}
}
