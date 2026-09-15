package runtime

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

func TestTodoListItemUsesProvidedIconProfile(t *testing.T) {
	item := todoListItem{
		item:  tododomain.Item{ID: "1", Text: "Ship icons", Status: tododomain.StatusInProgress},
		icons: tuistyle.UnicodeIcons,
	}
	if got := ansi.Strip(item.Title()); !strings.Contains(got, strings.TrimSpace(tuistyle.UnicodeIcons.TodoActive)) {
		t.Fatalf("todo title %q does not contain Nerd active glyph", got)
	}
}

func TestOpenTodoPaneCapturesRuntimeIconProfile(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.icons = tuistyle.UnicodeIcons
	_ = m.openTodoPane()

	view, _ := m.panes.bottom.find(todoInspectViewID).(*todoPaneView)
	if view == nil {
		t.Fatal("todo pane was not opened")
	}
	if view.icons != tuistyle.UnicodeIcons {
		t.Fatalf("todo pane icons = %#v, want Nerd profile", view.icons)
	}
}
