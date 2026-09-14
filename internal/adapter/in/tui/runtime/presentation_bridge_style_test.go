package runtime

import (
	"reflect"
	"testing"

	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

func TestPaneSectionUsesSemanticPaneTitle(t *testing.T) {
	rows := paneSection("Tasks", []string{"row"}, "", "", 80)
	if len(rows) == 0 {
		t.Fatal("paneSection returned no rows")
	}
	want := tuistyle.PaneTitleStyle.Render("Tasks")
	if rows[0] != want {
		t.Fatalf("pane title = %q, want semantic pane title %q", rows[0], want)
	}
}

func TestPaneToneColorDelegatesToSemanticTone(t *testing.T) {
	for _, tone := range []panecommon.Tone{
		panecommon.ToneAssistant,
		panecommon.ToneUser,
		panecommon.ToneWarning,
		panecommon.ToneError,
	} {
		if !reflect.DeepEqual(paneToneColor(tone), panecommon.ToneColor(tone)) {
			t.Fatalf("tone %v did not resolve through pane common semantic mapping", tone)
		}
	}
}
