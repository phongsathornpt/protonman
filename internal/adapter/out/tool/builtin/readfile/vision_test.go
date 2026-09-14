package readfile

import (
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestVisionTargetDimensions(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		width, height         int
		wantWidth, wantHeight int
	}{
		{
			name:       "small fits inside budget",
			width:      800,
			height:     600,
			wantWidth:  800,
			wantHeight: 600,
		},
		{
			name:       "dimension exceeds max 1568",
			width:      4000,
			height:     2000,
			wantWidth:  1568,
			wantHeight: 784,
		},
		{
			name:       "pixels exceed max 1.6M",
			width:      2000,
			height:     2000,
			wantWidth:  1264,
			wantHeight: 1264,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := visionTargetDimensions(tc.width, tc.height)
			if gotW != tc.wantWidth || gotH != tc.wantHeight {
				t.Fatalf("visionTargetDimensions(%d, %d) = (%d, %d), want (%d, %d)", tc.width, tc.height, gotW, gotH, tc.wantWidth, tc.wantHeight)
			}
			if int64(gotW)*int64(gotH) > maxVisionPixels {
				t.Fatalf("pixel area %d exceeds max %d", gotW*gotH, maxVisionPixels)
			}
			if gotW > maxVisionDimension || gotH > maxVisionDimension {
				t.Fatalf("dimension (%d, %d) exceeds max %d", gotW, gotH, maxVisionDimension)
			}
		})
	}
}

func TestEstimateVisionTokens(t *testing.T) {
	est1 := estimateVisionTokens(10, 10)
	if est1.OpenAI != 255 || est1.Anthropic != 1 {
		t.Fatalf("estimateVisionTokens(10, 10) = %+v, want OpenAI 255, Anthropic 1", est1)
	}

	est2 := estimateVisionTokens(512, 512)
	if est2.OpenAI != 255 || est2.Anthropic != 350 {
		t.Fatalf("estimateVisionTokens(512, 512) = %+v, want OpenAI 255, Anthropic 350", est2)
	}

	est3 := estimateVisionTokens(1568, 784)
	if est3.OpenAI != 1445 || est3.Anthropic != 1639 {
		t.Fatalf("estimateVisionTokens(1568, 784) = %+v, want OpenAI 1445, Anthropic 1639", est3)
	}
}

func TestFlattenToOpaqueCompositesOverWhite(t *testing.T) {
	// Create a 4x4 image: half transparent red, half fully transparent
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: 128}) // 50% transparent red
		}
	}
	for y := 2; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: 0}) // fully transparent
		}
	}

	flattened := flattenToOpaque(img, img.Bounds())
	if flattened.Bounds() != img.Bounds() {
		t.Fatalf("bounds mismatch: %v vs %v", flattened.Bounds(), img.Bounds())
	}

	// Bottom row (was fully transparent) should now be pure white (255, 255, 255, 255)
	c := flattened.RGBAAt(0, 3)
	if c.R != 255 || c.G != 255 || c.B != 255 || c.A != 255 {
		t.Fatalf("expected white pixel for transparent region, got %+v", c)
	}
}

func TestReadImagePopulatesImageAttachment(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	path := filepath.Join(ws.Root(), "vision.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 50, B: 50, A: 255})
		}
	}
	if err := png.Encode(file, img); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "vision", "read", map[string]any{"path": "vision.png", "view": "image"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.Image == nil {
		t.Fatal("expected result.Image to be populated")
	}
	if result.Image.Width != 10 || result.Image.Height != 10 {
		t.Fatalf("unexpected image attachment: %+v", result.Image)
	}
	decoded, err := base64.StdEncoding.DecodeString(result.Image.Data)
	if err != nil || len(decoded) == 0 {
		t.Fatalf("failed to decode base64 data: %v", err)
	}
}
