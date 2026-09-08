package tui

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestPickerRenderDoesNotMutateNavigationState(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(80, 24)

	t.Run("model selector", func(t *testing.T) {
		v := &modelSelectPaneView{index: 99, offset: 77, models: []model.RemoteModel{{ID: "one"}, {ID: "two"}}}
		beforeIndex, beforeOffset := v.index, v.offset
		_ = v.Render(m)
		if v.index != beforeIndex || v.offset != beforeOffset {
			t.Fatalf("Render mutated navigation: index %d→%d offset %d→%d", beforeIndex, v.index, beforeOffset, v.offset)
		}
	})
}
