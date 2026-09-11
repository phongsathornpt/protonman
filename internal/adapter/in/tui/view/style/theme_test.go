package style

import (
	"fmt"
	"image/color"
	"math"
	"reflect"
	"testing"
)

func TestSemanticStylesUseThemePalette(t *testing.T) {
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"assistant", AssistantStyle.GetForeground(), ColorTextPrimary},
		{"body", BodyStyle.GetForeground(), ColorTextPrimary},
		{"tool target", ToolTargetStyle.GetForeground(), ColorTextSecondary},
		{"diff hunk", DiffHunkStyle.GetForeground(), ColorTextSecondary},
		{"markdown heading", MarkdownHeadingStyle.GetForeground(), ColorTextPrimary},
		{"markdown bullet", MarkdownBulletStyle.GetForeground(), ColorTextSecondary},
	}
	for _, tt := range tests {
		if !reflect.DeepEqual(tt.got, tt.want) {
			t.Errorf("%s foreground = %#v, want %#v", tt.name, tt.got, tt.want)
		}
	}
}

// darkCanvas approximates the terminal background the theme is calibrated for.
const darkCanvasHex = "#0d0d0d"

func channelLuminance(value float64) float64 {
	value /= 255
	if value <= 0.04045 {
		return value / 12.92
	}
	return math.Pow((value+0.055)/1.055, 2.4)
}

func rgbChannels(c color.Color) (float64, float64, float64) {
	rgba := color.RGBAModel.Convert(c).(color.RGBA)
	return float64(rgba.R), float64(rgba.G), float64(rgba.B)
}

func relativeLuminance(c color.Color) float64 {
	red, green, blue := rgbChannels(c)
	return 0.2126*channelLuminance(red) + 0.7152*channelLuminance(green) + 0.0722*channelLuminance(blue)
}

func luminanceFromHex(hex string) float64 {
	var rgba color.RGBA
	if _, err := fmt.Sscanf(hex, "#%02x%02x%02x", &rgba.R, &rgba.G, &rgba.B); err != nil {
		return 0
	}
	return relativeLuminance(rgba)
}

func contrastRatio(foreground color.Color, backgroundHex string) float64 {
	first, second := relativeLuminance(foreground), luminanceFromHex(backgroundHex)
	if first < second {
		first, second = second, first
	}
	return (first + 0.05) / (second + 0.05)
}

// TestPaletteMeetsContrastFloor guards the accessibility calibration: every
// token that renders text must clear WCAG AA (4.5:1) against the dark canvas.
func TestPaletteMeetsContrastFloor(t *testing.T) {
	textTokens := map[string]color.Color{
		"text primary":   ColorTextPrimary,
		"text secondary": ColorTextSecondary,
		"text tertiary":  ColorTextTertiary,
		"primary":        ColorPrimary,
		"primary hover":  ColorPrimaryHover,
		"success":        ColorSuccess,
		"warning":        ColorWarning,
		"danger":         ColorDanger,
	}
	for name, token := range textTokens {
		if ratio := contrastRatio(token, darkCanvasHex); ratio < 4.5 {
			t.Errorf("%s contrast on %s = %.2f:1, want >= 4.5:1", name, darkCanvasHex, ratio)
		}
	}
}

// TestTextRampPreservesHierarchy guards the grey test: lightness must fall
// monotonically from primary to secondary to tertiary so the ramp survives
// without hue.
func TestTextRampPreservesHierarchy(t *testing.T) {
	primary := relativeLuminance(ColorTextPrimary)
	secondary := relativeLuminance(ColorTextSecondary)
	tertiary := relativeLuminance(ColorTextTertiary)
	if !(primary > secondary && secondary > tertiary) {
		t.Fatalf("text ramp collapsed: primary=%.3f secondary=%.3f tertiary=%.3f", primary, secondary, tertiary)
	}
}

// TestStateColorsCarryComparableLegibility keeps error, warning, and success
// within a narrow band, so a failure never reads dimmer than a success.
func TestStateColorsCarryComparableLegibility(t *testing.T) {
	success := relativeLuminance(ColorSuccess)
	warning := relativeLuminance(ColorWarning)
	danger := relativeLuminance(ColorDanger)
	lowest, highest := success, success
	for _, value := range []float64{warning, danger} {
		lowest = math.Min(lowest, value)
		highest = math.Max(highest, value)
	}
	if ratio := highest / lowest; ratio > 1.6 {
		t.Fatalf("state color legibility spread too wide (%.2fx): success=%.3f warning=%.3f danger=%.3f", ratio, success, warning, danger)
	}
}
