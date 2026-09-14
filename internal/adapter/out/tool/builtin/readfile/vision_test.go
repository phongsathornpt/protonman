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
		name                 string
		width, height        int
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
			name:       "dimension exceeds max 2048",
			width:      4000,
			height:     2000,
			wantWidth:  2048,
			wantHeight: 1024,
		},
		{
			name:       "patches exceed max 2500",
			width:      2000,
			height:     2000, // 63*63 = 3969 patches > 2500
			wantWidth:  1600,
			wantHeight: 1600,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := visionTargetDimensions(tc.width, tc.height, defaultMaxVisionDimension, defaultMaxVisionPatches)
			if gotW != tc.wantWidth || gotH != tc.wantHeight {
				t.Fatalf("visionTargetDimensions(%d, %d) = (%d, %d), want (%d, %d)", tc.width, tc.height, gotW, gotH, tc.wantWidth, tc.wantHeight)
			}
			if visionPatchCount(gotW, gotH) > defaultMaxVisionPatches {
				t.Fatalf("patch count %d exceeds max %d", visionPatchCount(gotW, gotH), defaultMaxVisionPatches)
			}
		})
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
	if result.Image.MIMEType != "image/png" || result.Image.Width != 10 || result.Image.Height != 10 {
		t.Fatalf("unexpected image attachment: %+v", result.Image)
	}
	decoded, err := base64.StdEncoding.DecodeString(result.Image.Data)
	if err != nil || len(decoded) == 0 {
		t.Fatalf("failed to decode base64 data: %v", err)
	}
}
