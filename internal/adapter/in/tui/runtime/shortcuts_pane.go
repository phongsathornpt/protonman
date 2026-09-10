package runtime

import tea "charm.land/bubbletea/v2"

const shortcutsViewID = "shortcuts"

type shortcutsPaneView struct{}

func (*shortcutsPaneView) ID() string                             { return shortcutsViewID }
func (*shortcutsPaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func (*shortcutsPaneView) Render(ctx paneRenderContext) string {
	rows := []string{
		userStyle.Render("enter") + mutedStyle.Render("  Send message"),
		userStyle.Render("ctrl+j") + mutedStyle.Render("  New line"),
		userStyle.Render("ctrl+p") + mutedStyle.Render("  Switch model"),
		userStyle.Render("ctrl+t") + mutedStyle.Render("  Transcript"),
		userStyle.Render("ctrl+o") + mutedStyle.Render("  Tasks"),
		userStyle.Render("ctrl+s") + mutedStyle.Render("  Skills"),
		userStyle.Render("shift+tab") + mutedStyle.Render("  Cycle mode"),
		userStyle.Render("ctrl+c") + mutedStyle.Render("  Cancel or quit"),
	}
	help := paneKeyboardHelp(ctx.width-4, "esc/?", "Go Back")
	return renderModalRows(ctx, accentAssistant, paneSection("Shortcuts", rows, help, "", ctx.width))
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
