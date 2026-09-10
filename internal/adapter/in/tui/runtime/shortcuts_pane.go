package runtime

import tea "charm.land/bubbletea/v2"

const shortcutsViewID = "shortcuts"

type shortcutsPaneView struct{}

func (*shortcutsPaneView) ID() string                             { return shortcutsViewID }
func (*shortcutsPaneView) PresentationMode() panePresentationMode { return paneOverlay }

func (*shortcutsPaneView) Render(ctx paneRenderContext) string {
	rows := []string{
		brandStyle.Render("Shortcuts"),
		mutedStyle.Render("enter send · ctrl+j newline"),
		mutedStyle.Render("ctrl+p model setup · ctrl+t transcript"),
		mutedStyle.Render("shift+tab mode · ctrl+o todos · ctrl+s skills"),
		mutedStyle.Render("ctrl+c cancel or quit"),
		mutedStyle.Render("esc or ? close"),
	}
	return renderModalRows(ctx, accentAssistant, rows)
}

func (*shortcutsPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch message.String() {
	case "esc", "?", "q", "enter":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: shortcutsViewID}}
	default:
		return paneKeyResult{handled: true}
	}
}

func (m *bubbleModel) openShortcutsPane() {
	if m.panes.bottom.has(shortcutsViewID) {
		m.panes.bottom.remove(shortcutsViewID)
		m.requestRelayout()
		return
	}
	m.panes.bottom.push(&shortcutsPaneView{})
	m.requestRelayout()
}
