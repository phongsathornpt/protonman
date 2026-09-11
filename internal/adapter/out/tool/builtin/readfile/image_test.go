package readfile

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/gif"
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

func TestReadFileExplicitImageViewRejectsNonImage(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	if err := os.WriteFile(filepath.Join(ws.Root(), "notes.txt"), []byte("not an image\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := New(ws).Execute(context.Background(), newJSONCall(t, "image-text", "read", map[string]any{
		"path": "notes.txt", "view": "image",
	}))
	if err == nil || !strings.Contains(err.Error(), "is not a supported image") {
		t.Fatalf("Execute() error = %v, want supported-image rejection", err)
	}
}

func TestImageSampleGridHonorsBudgetForExtremeAspectRatios(t *testing.T) {
	for _, tc := range []struct {
		width, height int
	}{
		{width: maxImagePixels, height: 1},
		{width: 1, height: maxImagePixels},
		{width: 4096, height: 3072},
		{width: 512, height: 512},
	} {
		cols, rows := imageSampleGrid(tc.width, tc.height, maxImageSamples)
		if cols < 1 || rows < 1 || cols > tc.width || rows > tc.height {
			t.Fatalf("grid %dx%d for %dx%d is invalid", cols, rows, tc.width, tc.height)
		}
		if cols*rows > maxImageSamples {
			t.Fatalf("grid %dx%d samples %d pixels, want <= %d", cols, rows, cols*rows, maxImageSamples)
		}
	}
}

func TestReadImageReportsExactOpacityOutsideSamplingGrid(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	path := filepath.Join(ws.Root(), "alpha.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 100, G: 120, B: 140, A: 255})
		}
	}
	img.SetNRGBA(511, 511, color.NRGBA{R: 255, A: 0})
	if err := png.Encode(file, img); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "alpha", "read", map[string]any{"path": "alpha.png", "view": "image"}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Metadata struct {
			Opaque bool `json:"opaque"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(result.StructuredOutput, &got); err != nil {
		t.Fatal(err)
	}
	if got.Metadata.Opaque {
		t.Fatal("opaque=true for image containing a transparent pixel")
	}
}

func TestReadImageRejectsOversizedEncodedFileBeforeDecode(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	path := filepath.Join(ws.Root(), "oversized.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Truncate(maxEncodedImageBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = New(ws).Execute(context.Background(), newJSONCall(t, "oversized", "read", map[string]any{"path": "oversized.png", "view": "image"}))
	if err == nil || !strings.Contains(err.Error(), "safe decode limit") {
		t.Fatalf("Execute() error = %v, want encoded-size rejection", err)
	}
}

func TestStratifiedSamplingDoesNotCollapseCheckerboard(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			if (x+y)%2 == 0 {
				img.SetNRGBA(x, y, color.NRGBA{A: 255})
			} else {
				img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	cols, rows := imageSampleGrid(512, 512, maxImageSamples)
	seenDark, seenLight := false, false
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			x := stratifiedCoordinate(512, cols, col, row, 0x9e3779b97f4a7c15)
			y := stratifiedCoordinate(512, rows, row, col, 0xbf58476d1ce4e5b9)
			r, _, _, _ := img.At(x, y).RGBA()
			seenDark = seenDark || r == 0
			seenLight = seenLight || r == 0xffff
		}
	}
	if !seenDark || !seenLight {
		t.Fatalf("stratified checkerboard sample dark=%v light=%v", seenDark, seenLight)
	}
}

func TestContextReaderStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader := &contextReader{ctx: ctx, reader: strings.NewReader("image-bytes")}
	buf := make([]byte, 5)
	if _, err := reader.Read(buf); err != nil {
		t.Fatalf("initial read: %v", err)
	}
	cancel()
	if _, err := reader.Read(buf); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read error = %v, want context.Canceled", err)
	}
}

func TestReadImageIgnoresFullyTransparentPixelsInVisualStats(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	path := filepath.Join(ws.Root(), "transparent.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	for x := 0; x < 3; x++ {
		img.SetNRGBA(x, 0, color.NRGBA{R: 255, A: 0})
	}
	img.SetNRGBA(3, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(file, img); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "transparent", "read", map[string]any{"path": "transparent.png", "view": "image"}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Analysis struct {
			Brightness struct {
				Count int `json:"count"`
			} `json:"brightness"`
			DominantColors []dominantColor `json:"dominant_colors"`
		} `json:"analysis"`
	}
	if err := json.Unmarshal(result.StructuredOutput, &got); err != nil {
		t.Fatal(err)
	}
	if got.Analysis.Brightness.Count != 1 {
		t.Fatalf("visible brightness samples = %d, want 1", got.Analysis.Brightness.Count)
	}
	if len(got.Analysis.DominantColors) != 1 || got.Analysis.DominantColors[0].RGB[0] < 240 {
		t.Fatalf("dominant colors = %#v, want visible red only", got.Analysis.DominantColors)
	}
}

func TestReadGIFDeclaresFirstFrameAnalysisScope(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	path := filepath.Join(ws.Root(), "animated.gif")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	second.SetColorIndex(0, 0, 1)
	if err := gif.EncodeAll(file, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{1, 1}}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "gif", "read", map[string]any{"path": "animated.gif", "view": "image"}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Metadata struct {
			AnalysisScope string `json:"analysis_scope"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(result.StructuredOutput, &got); err != nil {
		t.Fatal(err)
	}
	if got.Metadata.AnalysisScope != "first_frame" {
		t.Fatalf("analysis scope = %q, want first_frame", got.Metadata.AnalysisScope)
	}
}

func TestReadFileAutoAnalyzesWebP(t *testing.T) {
	ws := newTestWorkspace(t, nil)
	fixture, err := os.ReadFile("testdata/red.webp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws.Root(), "red.webp"), fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := New(ws).Execute(context.Background(), newJSONCall(t, "webp", "read", map[string]any{"path": "red.webp"}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(result.Output, "image webp 1x1") {
		t.Fatalf("Output = %q", result.Output)
	}
	var got struct {
		Kind     string `json:"kind"`
		MIMEType string `json:"mime_type"`
		Metadata struct {
			Format string `json:"format"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(result.StructuredOutput, &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "image" || got.MIMEType != "image/webp" || got.Metadata.Format != "webp" || got.Metadata.Width != 1 || got.Metadata.Height != 1 {
		t.Fatalf("structured WebP result = %+v", got)
	}
}
