//go:build desktop || desktop_gio

package settings

import (
	"image"
	"image/color"
	"testing"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func TestTabSelectionAndModalLifecycle(t *testing.T) {
	component := New()
	if component.IsOpen() {
		t.Fatal("settings modal should start closed")
	}
	component.SetActiveTab(AgentsTab)
	component.Show()
	if !component.IsOpen() || component.ActiveTab() != AgentsTab {
		t.Fatalf("modal state = open:%v tab:%d, want open on ACP agents", component.IsOpen(), component.ActiveTab())
	}
	component.SetActiveTab(99)
	if component.ActiveTab() != AgentsTab {
		t.Fatalf("out-of-range tab = %d, want clamped to %d", component.ActiveTab(), AgentsTab)
	}
	component.Close()
	if component.IsOpen() {
		t.Fatal("Close should hide the settings modal")
	}
}

func TestGeneralThemeChoiceCallsShellCallback(t *testing.T) {
	component := New()
	var selected string
	chrome := Chrome{
		Label: func(layout.Context, string, unit.Sp, font.Weight, color.NRGBA, int) layout.Dimensions {
			return layout.Dimensions{}
		},
		RoundedSurface: func(gtx layout.Context, _ unit.Dp, _ color.NRGBA, child layout.Widget) layout.Dimensions {
			return child(gtx)
		},
		BorderSurface: func(gtx layout.Context, _ unit.Dp, _, _ color.NRGBA, _ int, child layout.Widget) layout.Dimensions {
			return child(gtx)
		},
		Inset: func(gtx layout.Context, inset layout.Inset, child layout.Widget) layout.Dimensions {
			return inset.Layout(gtx, child)
		},
		Divider: func(layout.Context) layout.Dimensions { return layout.Dimensions{} },
	}
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(600, 700)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Unix(1, 0)}
	input := GeneralInput{Theme: "dark", Chrome: chrome, OnSetTheme: func(theme string) { selected = theme }}
	component.LayoutGeneral(gtx, input)
	button := component.themeButtons["slate-light"]
	if button == nil {
		t.Fatal("theme choice button was not initialized")
	}
	button.Click()
	gtx.Ops = new(op.Ops)
	component.LayoutGeneral(gtx, input)
	if selected != "slate-light" {
		t.Fatalf("selected theme = %q, want slate-light", selected)
	}
}

func TestAgentSelectorOwnsWidgetsAndClosesOnSelection(t *testing.T) {
	component := New()
	component.SyncAgentChoices([]string{"reviewer"})
	component.ToggleAgentSelector()
	var selected string
	chrome := Chrome{
		Label: func(layout.Context, string, unit.Sp, font.Weight, color.NRGBA, int) layout.Dimensions {
			return layout.Dimensions{}
		},
		RoundedSurface: func(gtx layout.Context, _ unit.Dp, _ color.NRGBA, child layout.Widget) layout.Dimensions {
			return child(gtx)
		},
		BorderSurface: func(gtx layout.Context, _ unit.Dp, _, _ color.NRGBA, _ int, child layout.Widget) layout.Dimensions {
			return child(gtx)
		},
		Inset: func(gtx layout.Context, inset layout.Inset, child layout.Widget) layout.Dimensions {
			return inset.Layout(gtx, child)
		},
	}
	input := AgentSelectorInput{
		Profiles: []AgentChoice{{ID: "reviewer", DisplayName: "Reviewer", Connection: "connected"}},
		Chrome:   chrome, OnSelect: func(id string) { selected = id },
	}
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(500, 100)), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Now: time.Unix(1, 0)}
	component.LayoutAgentSelector(gtx, input)
	button := component.agentButtons["reviewer"]
	if button == nil {
		t.Fatal("agent choice button was not initialized")
	}
	button.Click()
	gtx.Ops = new(op.Ops)
	component.LayoutAgentSelector(gtx, input)
	if selected != "reviewer" || component.AgentSelectorVisible() {
		t.Fatalf("selection = %q, selector visible = %v; want reviewer and closed", selected, component.AgentSelectorVisible())
	}
}
