//go:build desktop || desktop_gio

package settings

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

var (
	testShaper        = text.NewShaper()
	testTheme         = material.NewTheme()
	testSelectionCall = func() op.CallOp { m := op.Record(new(op.Ops)); return m.Stop() }()
	testCursorCall    = func() op.CallOp { m := op.Record(new(op.Ops)); return m.Stop() }()
)

func agentsTestChrome() Chrome {
	button := func(gtx layout.Context, target *widget.Clickable, _ string, enabled bool, action func()) layout.Dimensions {
		if enabled && target.Clicked(gtx) && action != nil {
			action()
		}
		return target.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} })
	}
	return Chrome{
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
