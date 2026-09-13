package runtime

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestVisualStyleFullFrameResponsiveModes(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		height int
	}{
		{name: "normal", width: 80, height: 24},
		{name: "compact", width: 32, height: 18},
		{name: "tiny", width: 20, height: 12},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/composer", func(t *testing.T) {
			m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
			assertBubbleViewFits(t, m, tc.width, tc.height)
		})
		t.Run(tc.name+"/shortcuts", func(t *testing.T) {
			m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
			m.openShortcutsPane()
			assertBubbleViewFits(t, m, tc.width, tc.height)
		})
	}
}

func TestVisualStyleDenyComposerFitsResponsiveModes(t *testing.T) {
	for _, size := range []struct{ width, height int }{
		{80, 24},
		{32, 18},
		{20, 12},
	} {
		m := newTestBubbleModel(t, permission.ModeDeny, emptyTodoItems())
		assertBubbleViewFits(t, m, size.width, size.height)
	}
}
