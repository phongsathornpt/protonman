package runtime

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type shortcutRoutingPane struct {
	id      string
	mode    panePresentationMode
	handled int
}

func (v *shortcutRoutingPane) ID() string                             { return v.id }
func (v *shortcutRoutingPane) Render(paneRenderContext) string        { return "" }
func (v *shortcutRoutingPane) PresentationMode() panePresentationMode { return v.mode }
func (v *shortcutRoutingPane) HandlePaneKey(_ paneRenderContext, _ tea.KeyPressMsg) paneKeyResult {
	v.handled++
	return paneKeyResult{handled: true}
}

func TestGlobalShortcutsPreemptNonBlockingPane(t *testing.T) {
	tests := []struct {
		name  string
		key   tea.KeyPressMsg
		check func(*testing.T, *bubbleModel)
	}{
		{"permission", testShiftTab(), func(t *testing.T, m *bubbleModel) {
			if !m.planMode {
				t.Fatal("shift+tab did not cycle permission")
			}
		}},
		{"model", testCtrl('p'), func(t *testing.T, m *bubbleModel) {
			if !m.panes.bottom.has(modelSetupViewID) {
				t.Fatal("ctrl+p did not open model setup")
			}
		}},
		{"skills", testCtrl('s'), func(t *testing.T, m *bubbleModel) {
			if !m.panes.bottom.has(skillsViewID) {
				t.Fatal("ctrl+s did not open skills")
			}
		}},
		{"todo", testCtrl('o'), func(t *testing.T, m *bubbleModel) {
			if !m.panes.bottom.has(todoInspectViewID) {
				t.Fatal("ctrl+o did not open todo")
			}
		}},
		{"transcript", testCtrl('t'), func(t *testing.T, m *bubbleModel) {
			if !m.panes.showTranscript {
				t.Fatal("ctrl+t did not open transcript")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestSkillsModel(t, 1)
			pane := &shortcutRoutingPane{id: "non-blocking-test", mode: paneBelowComposer}
			m.panes.bottom.push(pane)
			updated, _ := m.Update(tt.key)
			m = updated.(*bubbleModel)
			tt.check(t, m)
			if pane.handled != 0 {
				t.Fatalf("global key leaked to pane: handled=%d", pane.handled)
			}
		})
	}
}

func TestBlockingPaneOwnsGlobalShortcuts(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	pane := &shortcutRoutingPane{id: "blocking-test", mode: paneBlocking}
	m.panes.bottom.push(pane)
	for _, key := range []tea.KeyPressMsg{testShiftTab(), testCtrl('p'), testCtrl('s'), testCtrl('o'), testCtrl('t')} {
		updated, _ := m.Update(key)
		m = updated.(*bubbleModel)
	}
	if pane.handled != 5 {
		t.Fatalf("blocking pane handled=%d, want 5", pane.handled)
	}
	if m.planMode || m.panes.showTranscript || m.panes.bottom.has(modelSetupViewID) || m.panes.bottom.has(skillsViewID) || m.panes.bottom.has(todoInspectViewID) {
		t.Fatal("global shortcut escaped blocking pane")
	}
}

func TestLegacyShortcutAliasesAreInactive(t *testing.T) {
	m := newTestSkillsModel(t, 1)
	m.appendLine("sentinel transcript")
	before := m.historyState.Raw()
	updated, _ := m.Update(testCtrl('l'))
	m = updated.(*bubbleModel)
	if got := m.historyState.Raw(); got != before || !strings.Contains(got, "sentinel transcript") {
		t.Fatalf("ctrl+l still clears transcript: %q", got)
	}
	updated, _ = m.Update(testAltText("m"))
	m = updated.(*bubbleModel)
	if m.panes.bottom.has(modelSetupViewID) {
		t.Fatal("legacy alt+m alias still opens model setup")
	}
}
