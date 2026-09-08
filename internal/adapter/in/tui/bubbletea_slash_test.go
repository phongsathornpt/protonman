// Code grouped by TUI behavior boundary; shared fixtures live in bubbletea_helpers_test.go.
package tui

import (
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"strings"
	"testing"
)

func TestSlashDropdownFiltersAndTabAccepts(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.prompt.SetValue("/he")
	if !model.slashOpen() {
		t.Fatal("slash dropdown did not open for /he")
	}
	matches := model.slashMatches()
	if len(matches) != 1 || matches[0].Name != "help" {
		t.Fatalf("slash matches = %#v, want help", matches)
	}
	applied, command := model.acceptSlash(false)
	if !applied || command != nil {
		t.Fatalf("tab accept applied=%v command=%v", applied, command)
	}
	if got := model.prompt.Value(); got != "/help" {
		t.Fatalf("tab accept value = %q, want /help", got)
	}
}

func TestColonAliasDispatchesHelp(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.prompt.SetValue(":help")
	if command := model.submit(); command != nil {
		t.Fatalf("colon help command = %v, want nil", command)
	}
	if !strings.Contains(plainTranscript(model), "/call") {
		t.Fatalf("colon alias did not render help: %q", plainTranscript(model))
	}
}
