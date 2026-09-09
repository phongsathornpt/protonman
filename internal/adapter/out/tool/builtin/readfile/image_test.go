package readfile

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileAutoAnalyzesPNG(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	path := filepath.Join(ws.Root(), "screen.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 80, G: 80, B: 112, A: 255})
		}
	}
	if err := png.Encode(file, img); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "image", "read", map[string]any{"path": "screen.png"}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "image png 4x2") {
		t.Fatalf("Output = %q", result.Output)
	}
	var got struct {
		Kind     string `json:"kind"`
		Metadata struct {
			Format string `json:"format"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"metadata"`
		Analysis struct {
			Samples int `json:"samples"`
		} `json:"analysis"`
	}
	if err := json.Unmarshal(result.StructuredOutput, &got); err != nil {
		t.Fatalf("structured output: %v", err)
	}
	if got.Kind != "image" || got.Metadata.Format != "png" || got.Metadata.Width != 4 || got.Metadata.Height != 2 {
		t.Fatalf("structured image result = %+v", got)
	}
	if got.Analysis.Samples != 8 {
		t.Fatalf("samples = %d, want 8", got.Analysis.Samples)
	}
}

func TestReadFileAutoReturnsBinaryMetadata(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	if err := os.WriteFile(filepath.Join(ws.Root(), "blob.bin"), []byte{0, 1, 2, 3, 4, 5}, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "binary", "read", map[string]any{"path": "blob.bin"}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "binary") || len(result.StructuredOutput) == 0 {
		t.Fatalf("binary result = %+v", result)
	}
}

func TestImageDimensionsWithinSafeAnalysisLimit(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
		want          bool
	}{
		{name: "zero-width", width: 0, height: 10, want: false},
		{name: "at-limit", width: 4096, height: 3072, want: true},
		{name: "over-limit", width: 4096, height: 3073, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := imageDimensionsWithinLimit(tc.width, tc.height); got != tc.want {
				t.Fatalf("imageDimensionsWithinLimit(%d, %d) = %v, want %v", tc.width, tc.height, got, tc.want)
			}
		})
	}
}
