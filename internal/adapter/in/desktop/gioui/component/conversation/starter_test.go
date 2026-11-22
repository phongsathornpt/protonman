//go:build desktop || desktop_gio

package conversation

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

func starterTestChrome() StarterChrome {
	return StarterChrome{
		SurfaceContainerHigh:    color.NRGBA{A: 0xff},
		SurfaceContainerHighest: color.NRGBA{A: 0xff},
		OutlineVariant:          color.NRGBA{A: 0xff},
		Primary:                 color.NRGBA{A: 0xff},
		OnSurface:               color.NRGBA{A: 0xff},
		OnSurfaceVariant:        color.NRGBA{A: 0xff},
		BorderSurface: func(gtx layout.Context, _ unit.Dp, _, _ color.NRGBA, _ int, child layout.Widget) layout.Dimensions {
			return child(gtx)
		},
		Label: func(layout.Context, string, unit.Sp, font.Weight, color.NRGBA, int) layout.Dimensions {
			return layout.Dimensions{}
		},
		Icon: func(layout.Context, StarterIcon, unit.Dp, color.NRGBA) layout.Dimensions {
			return layout.Dimensions{}
		},
	}
}

// Clicking a starter card must seed the composer with that card's prompt, and a
// second click must replace the previous text rather than appending. Selecting a
// starter is a "start me off" gesture, so leftover text would silently corrupt
// the first real prompt.
func TestStarterCardSelectionReplacesComposerText(t *testing.T) {
	component := New(8)
	chrome := starterTestChrome()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
	}
	prompts := StarterPrompts()
	if len(prompts) < 3 {
		t.Fatalf("starter prompt set has %d entries, want at least 3", len(prompts))
	}

	for _, index := range []int{0, 2} {
		button := component.StarterButton(index)
		if button == nil {
			t.Fatalf("starter button %d was not created", index)
		}
		button.Click()
		var selected string
		component.LayoutStarterCard(gtx, index, chrome, func(prompt string) { selected = prompt })
		if selected != prompts[index].Prompt {
			t.Fatalf("starter %d selected %q, want %q", index, selected, prompts[index].Prompt)
		}
		component.Editor().SetText(selected)
		if got := component.Editor().Text(); got != prompts[index].Prompt {
			t.Fatalf("composer text after clicking card %d = %q, want %q", index, got, prompts[index].Prompt)
		}
	}
}

// An out-of-range index must render nothing rather than panic: the shell can ask
// for a card index from a list it does not own.
func TestStarterCardIgnoresOutOfRangeIndex(t *testing.T) {
	component := New(8)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
	}
	called := false
	for _, index := range []int{-1, len(StarterPrompts()) + 5} {
		dims := component.LayoutStarterCard(gtx, index, starterTestChrome(), func(string) { called = true })
		if dims != (layout.Dimensions{}) {
			t.Fatalf("out-of-range starter %d produced dims %v, want zero", index, dims)
		}
	}
	if called {
		t.Fatal("an out-of-range starter index must not report a selection")
	}
}
