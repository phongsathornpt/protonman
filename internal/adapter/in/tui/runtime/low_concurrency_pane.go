package runtime

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

const lowConcurrencyViewID = "low-concurrency"

type lowConcurrencyPaneView struct {
	index     int
	effective bool
	modelID   string
}

func (*lowConcurrencyPaneView) ID() string                             { return lowConcurrencyViewID }
func (*lowConcurrencyPaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func (v *lowConcurrencyPaneView) Render(ctx paneRenderContext) string {
	choices := []struct {
		label string
		desc  string
	}{
		{"Auto", "use provider/model recommendation"},
		{"On", "force low concurrency for this model"},
		{"Off", "disable low concurrency"},
	}
	rows := make([]string, 0, len(choices)+1)
	for i, choice := range choices {
		marker := "  "
		style := mutedStyle
		if i == v.index {
			marker = "> "
			style = userStyle
		}
		rows = append(rows, marker+style.Render(choice.label)+"  "+mutedStyle.Render(choice.desc))
	}
	state := "off"
	if v.effective {
		state = "on"
	}
	if v.modelID != "" {
		rows = append(rows, "", mutedStyle.Render(fmt.Sprintf("Effective  %s · %s", state, v.modelID)))
	} else {
		rows = append(rows, "", mutedStyle.Render("Effective  off · no active model"))
	}
	help := paneKeyboardHelp(ctx.width-4, "↑/↓", "Navigate", "enter", "Apply", "esc/q", "Go Back")
	return renderModalRows(ctx, accentAssistant, paneSection("Low Concurrency", rows, help, "", ctx.width))
}

func (v *lowConcurrencyPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch {
	case key.Matches(message, paneKeys.Close):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: lowConcurrencyViewID}}
	case key.Matches(message, paneKeys.Up):
		if v.index > 0 {
			v.index--
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneKeys.Down):
		if v.index < 2 {
			v.index++
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneKeys.Confirm):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionSetLowConcurrency, lowConcurrency: model.LowConcurrencySetting(v.index)}}
	default:
		return paneKeyResult{handled: true}
	}
}

func (m *bubbleModel) openLowConcurrencyPane() {
	if m.panes.bottom.has(lowConcurrencyViewID) {
		m.panes.bottom.remove(lowConcurrencyViewID)
		m.requestRelayout()
		return
	}
	m.panes.bottom.push(&lowConcurrencyPaneView{index: int(m.lowConcurrencyMode), effective: m.lowConcurrencyEffective(), modelID: m.activeModel})
	m.requestRelayout()
}
