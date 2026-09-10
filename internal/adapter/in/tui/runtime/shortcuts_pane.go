package runtime

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

const shortcutsViewID = "shortcuts"

type shortcutsPaneView struct{}

func (*shortcutsPaneView) ID() string                             { return shortcutsViewID }
func (*shortcutsPaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func shortcutRow(binding key.Binding) string {
	help := binding.Help()
	desc := help.Desc
	if desc != "" {
		desc = strings.ToUpper(desc[:1]) + desc[1:]
	}
	return userStyle.Render(help.Key) + mutedStyle.Render("  "+desc)
}

func (*shortcutsPaneView) Render(ctx paneRenderContext) string {
	keys := newBubbleKeyMap()
	rows := []string{
		shortcutRow(keys.Submit),
		shortcutRow(keys.Newline),
		shortcutRow(keys.ToggleModel),
		shortcutRow(keys.Transcript),
		shortcutRow(keys.ToggleTodo),
		shortcutRow(keys.ToggleSkills),
		shortcutRow(keys.CyclePermission),
		shortcutRow(keys.Quit),
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
