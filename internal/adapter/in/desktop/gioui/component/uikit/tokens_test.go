//go:build desktop || desktop_gio

package uikit

import (
	"image/color"
	"testing"
)

func TestInterpolateColor(t *testing.T) {
	black := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	white := color.NRGBA{R: 200, G: 200, B: 200, A: 255}

	atZero := InterpolateColor(black, white, 0.0)
	if atZero != black {
		t.Fatalf("InterpolateColor(black, white, 0) = %+v, want %+v", atZero, black)
	}

	atOne := InterpolateColor(black, white, 1.0)
	if atOne != white {
		t.Fatalf("InterpolateColor(black, white, 1) = %+v, want %+v", atOne, white)
	}

	atHalf := InterpolateColor(black, white, 0.5)
	if atHalf.R != 100 || atHalf.G != 100 || atHalf.B != 100 || atHalf.A != 255 {
		t.Fatalf("InterpolateColor(black, white, 0.5) = %+v, want (100, 100, 100, 255)", atHalf)
	}

	// Boundary clamping
	under := InterpolateColor(black, white, -0.5)
	if under != black {
		t.Fatalf("InterpolateColor underflow clamped = %+v, want %+v", under, black)
	}
	over := InterpolateColor(black, white, 1.5)
	if over != white {
		t.Fatalf("InterpolateColor overflow clamped = %+v, want %+v", over, white)
	}
}

func TestWithAlpha(t *testing.T) {
	c := color.NRGBA{R: 50, G: 100, B: 150, A: 200}

	atFull := WithAlpha(c, 1.0)
	if atFull != c {
		t.Fatalf("WithAlpha(c, 1) = %+v, want %+v", atFull, c)
	}

	atZero := WithAlpha(c, 0.0)
	if atZero.A != 0 {
		t.Fatalf("WithAlpha(c, 0) alpha = %d, want 0", atZero.A)
	}

	atHalf := WithAlpha(c, 0.5)
	if atHalf.A != 100 || atHalf.R != 50 || atHalf.G != 100 || atHalf.B != 150 {
		t.Fatalf("WithAlpha(c, 0.5) = %+v, want A=100 and RGB preserved", atHalf)
	}
}

func TestEaseOutCubic(t *testing.T) {
	if EaseOutCubic(0.0) != 0.0 {
		t.Fatalf("EaseOutCubic(0) = %f, want 0", EaseOutCubic(0.0))
	}
	if EaseOutCubic(1.0) != 1.0 {
		t.Fatalf("EaseOutCubic(1) = %f, want 1", EaseOutCubic(1.0))
	}
	// Ease out should be faster at start: at t=0.5, 1 - (0.5)^3 = 0.875
	half := EaseOutCubic(0.5)
	if half != 0.875 {
		t.Fatalf("EaseOutCubic(0.5) = %f, want 0.875", half)
	}
	// Clamping
	if EaseOutCubic(-0.2) != 0.0 {
		t.Fatalf("EaseOutCubic(-0.2) clamped = %f, want 0", EaseOutCubic(-0.2))
	}
	if EaseOutCubic(1.5) != 1.0 {
		t.Fatalf("EaseOutCubic(1.5) clamped = %f, want 1", EaseOutCubic(1.5))
	}
}
