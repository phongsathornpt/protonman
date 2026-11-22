//go:build desktop || desktop_gio

package uikit

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

var regenerateIconSheet = flag.Bool("regenerate-icon-sheet", false, "rewrite testdata/icon-sheet.png from the current icon geometry")

var iconSheetKinds = []IconKind{
	KindSidebar, KindInspector, KindSearch, KindAdd,
	KindSend, KindStop, KindClose,
	KindChevronDown, KindChevronRight, KindChevronUp,
	KindKebab, KindStar, KindSettings,
	KindCompose, KindFolder, KindArchive,
	KindDiscord, KindBrandLogo, KindTerminal,
	KindCheck, KindCopy, KindTrash,
}

// TestIconSheetRender pins every glyph to a rendered PNG sheet. Geometry edits
// must visibly change this artifact, and the refreshed file must be reviewed
// in the diff before it becomes the new contract.
func TestIconSheetRender(t *testing.T) {
	const cell, cols = 64, 8
	rows := (len(iconSheetKinds) + cols - 1) / cols
	width, height := cols*cell, rows*cell
	win, err := headless.NewWindow(width, height)
	if err != nil {
		t.Skipf("headless window unavailable: %v", err)
	}
	defer win.Release()

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(width, height)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Source:      router.Source(),
	}
	paper := color.NRGBA{R: 0xf6, G: 0xf8, B: 0xfa, A: 0xff}
	paint.FillShape(gtx.Ops, paper, clip.Rect{Max: image.Pt(width, height)}.Op())
	ink := color.NRGBA{R: 0x1f, G: 0x23, B: 0x28, A: 0xff}
	for i, kind := range iconSheetKinds {
		cx, cy := (i%cols)*cell, (i/cols)*cell
		stack := op.Offset(image.Pt(cx+16, cy+16)).Push(gtx.Ops)
		sub := gtx
		sub.Constraints = layout.Exact(image.Pt(32, 32))
		LayoutActionIcon(sub, kind, 32, ink)
		stack.Pop()
	}
	if err := win.Frame(gtx.Ops); err != nil {
		t.Fatalf("win.Frame failed: %v", err)
	}
	sheet := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := win.Screenshot(sheet); err != nil {
		t.Fatalf("win.Screenshot failed: %v", err)
	}

	const path = "testdata/icon-sheet.png"
	if *regenerateIconSheet {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create testdata directory: %v", err)
		}
		file, err := os.Create(path)
		if err != nil {
			t.Fatalf("create icon sheet: %v", err)
		}
		if err := png.Encode(file, sheet); err != nil {
			file.Close()
			t.Fatalf("encode icon sheet: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close icon sheet: %v", err)
		}
		t.Logf("rewrote %s", path)
		return
	}

	wantFile, err := os.Open(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (generate deliberately with -regenerate-icon-sheet after reviewing the geometry change)", path, err)
	}
	defer wantFile.Close()
	wantImg, err := png.Decode(wantFile)
	if err != nil {
		t.Fatalf("decode golden %s: %v", path, err)
	}
	if !wantImg.Bounds().Eq(sheet.Bounds()) {
		t.Fatalf("icon sheet bounds = %v, want %v", sheet.Bounds(), wantImg.Bounds())
	}
	var diffPixels int
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if sheet.RGBAAt(x, y) != wantImg.At(x, y) {
				diffPixels++
			}
		}
	}
	if diffPixels != 0 {
		t.Fatalf("%d pixels differ from %s (regenerate with -regenerate-icon-sheet only after reviewing the geometry change)", diffPixels, path)
	}
}
