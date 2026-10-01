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
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
	"github.com/phongsathornpt/protonman/internal/app"
)

var (
	testShaper = text.NewShaper()
	testTheme  = material.NewTheme()
	// The editor needs real recorded CallOps; a zero op.CallOp would panic inside
	// widget.Editor when it paints the selection and cursor.
	testSelectionCall = func() op.CallOp { m := op.Record(new(op.Ops)); return m.Stop() }()
	testCursorCall    = func() op.CallOp { m := op.Record(new(op.Ops)); return m.Stop() }()
)

// agentsTestChrome supplies no-op primitives so the agent editor's control flow
// runs without a real paint pipeline. Every callback the editor can reach is
// present, so a nil here would panic rather than silently skip a branch.
func agentsTestChrome() Chrome {
	button := func(gtx layout.Context, target *widget.Clickable, _ string, enabled bool, action func()) layout.Dimensions {
		if enabled && target.Clicked(gtx) && action != nil {
			action()
		}
		return target.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })
	}
	return Chrome{
		// Providers render their fields through material.Editor(chrome.Material, ...)
		// directly, so the stub theme must be present or it nil-derefs.
		Material: testTheme,
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
		Divider:    func(layout.Context) layout.Dimensions { return layout.Dimensions{} },
		PanelTitle: func(layout.Context, string) layout.Dimensions { return layout.Dimensions{} },
		Editor: func(gtx layout.Context, _ string, editor *widget.Editor, _ bool) layout.Dimensions {
			// A real material.Theme is required: widget.Editor reads its palette
			// and selection colors, so a zero value would nil-deref here.
			return editor.Layout(gtx, testShaper, font.Font{Typeface: font.Typeface("Go")}, unit.Sp(14), testSelectionCall, testCursorCall)
		},
		MiniIconButton: func(gtx layout.Context, target *widget.Clickable, _ string, _ color.NRGBA) layout.Dimensions {
			return target.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })
		},
		ActionIcon: func(layout.Context, uikit.Icon, unit.Dp, color.NRGBA) layout.Dimensions {
			return layout.Dimensions{}
		},
		Button:        button,
		PrimaryButton: button,
		DangerButton:  button,
	}
}

func agentsTestContext(now int64) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1180, 760)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(now, 0),
	}
}

func agentsTestInput() AgentsInput {
	return AgentsInput{
		Profiles: []app.ACPAgentProfile{
			{ID: "protonman", DisplayName: "Protonman", Command: "protonman", Args: []string{"--acp"}},
		},
		ActiveAgentID: "protonman",
		Chrome:        agentsTestChrome(),
	}
}

// The "+ Add ACP Agent" toggle opens the definition form and the form's close
// control closes it again. A toggle that only ever opens leaves the user unable
// to dismiss a form they opened by accident.
func TestAgentEditorToggleOpensAndCloseDismisses(t *testing.T) {
	component := New()
	input := agentsTestInput()

	component.LayoutAgents(agentsTestContext(1), input)
	if component.AgentWidgets().EditorVisible {
		t.Fatal("the agent editor must start closed")
	}

	// A Clickable reports Clicked on the frame after the click, so prime it once.
	component.AgentWidgets().FormToggleButton.Click()
	component.LayoutAgents(agentsTestContext(2), input)
	component.LayoutAgents(agentsTestContext(3), input)
	if !component.AgentWidgets().EditorVisible {
		t.Fatal("clicking the add-agent toggle did not open the editor")
	}

	component.AgentWidgets().CloseButton.Click()
	component.LayoutAgents(agentsTestContext(4), input)
	component.LayoutAgents(agentsTestContext(5), input)
	if component.AgentWidgets().EditorVisible {
		t.Fatal("the editor's close control did not dismiss the form")
	}
}

// A preset must fill the whole form. Clicking "OpenCode" and getting an empty
// command field would leave the user to fill in a command they just asked for.
func TestAgentPresetButtonsPopulateTheForm(t *testing.T) {
	cases := []struct {
		name        string
		button      func(*AgentWidgets) *widget.Clickable
		wantID      string
		wantName    string
		wantCommand string
	}{
		{"opencode", func(w *AgentWidgets) *widget.Clickable { return &w.PresetOpencodeButton }, "opencode", "OpenCode", "opencode"},
		{"cline", func(w *AgentWidgets) *widget.Clickable { return &w.PresetClineButton }, "cline", "Cline", "cline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			component := New()
			input := agentsTestInput()
			widgets := component.AgentWidgets()
			widgets.EditorVisible = true

			tc.button(widgets).Click()
			component.LayoutAgents(agentsTestContext(1), input)
			component.LayoutAgents(agentsTestContext(2), input)

			if got := widgets.IDEditor.Text(); got != tc.wantID {
				t.Fatalf("preset %s id = %q, want %q", tc.name, got, tc.wantID)
			}
			if got := widgets.NameEditor.Text(); got != tc.wantName {
				t.Fatalf("preset %s name = %q, want %q", tc.name, got, tc.wantName)
			}
			if got := widgets.CommandEditor.Text(); got != tc.wantCommand {
				t.Fatalf("preset %s command = %q, want %q", tc.name, got, tc.wantCommand)
			}
		})
	}
}

// While a save is in flight the agent controls must be inert. Re-submitting a
// profile mid-write would race two mutations against the same store.
func TestAgentEditorIsInertWhileSaving(t *testing.T) {
	component := New()
	input := agentsTestInput()
	input.Updating = true
	widgets := component.AgentWidgets()

	component.LayoutAgents(agentsTestContext(1), input)
	widgets.FormToggleButton.Click()
	component.LayoutAgents(agentsTestContext(2), input)
	component.LayoutAgents(agentsTestContext(3), input)

	if widgets.EditorVisible {
		t.Fatal("the add-agent toggle must not open the editor while a save is in flight")
	}
}
