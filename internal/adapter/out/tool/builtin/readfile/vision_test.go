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

func TestEstimateVisionTokensUsesProviderNeutralPatches(t *testing.T) {
	est1 := estimateVisionTokens(10, 10)
	if est1.OpenAI != 1 || est1.Anthropic != 1 {
		t.Fatalf("estimateVisionTokens(10, 10) = %+v, want one patch", est1)
	}

	est2 := estimateVisionTokens(512, 512)
	if est2.OpenAI != 256 || est2.Anthropic != 256 {
		t.Fatalf("estimateVisionTokens(512, 512) = %+v, want 256 patches", est2)
	}
}

func TestReadImagePopulatesSharedPreparedAttachment(t *testing.T) {
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
