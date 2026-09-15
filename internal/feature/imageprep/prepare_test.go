package imageprep

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func pngDataURLPayload(t *testing.T, width, height int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func TestOutputDimensionsMatchesPatchPolicy(t *testing.T) {
	policy := DefaultPolicy()
	for _, tc := range []struct {
		width, height int
		wantW, wantH  int
	}{
		{800, 600, 800, 600},
		{4000, 2000, 2048, 1024},
		{2000, 2000, 1600, 1600},
	} {
		gotW, gotH := OutputDimensions(tc.width, tc.height, policy)
		if gotW != tc.wantW || gotH != tc.wantH {
			t.Fatalf("OutputDimensions(%d,%d)=(%d,%d), want (%d,%d)", tc.width, tc.height, gotW, gotH, tc.wantW, tc.wantH)
		}
	}
}

func TestPreparePreservesSmallPNGBytes(t *testing.T) {
	data := pngDataURLPayload(t, 64, 64)
	prepared, err := Prepare("image/png", data, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Data != data {
		t.Fatal("small PNG was re-encoded despite fitting the prompt budget")
	}
	if prepared.Width != 64 || prepared.Height != 64 || prepared.Resized() {
		t.Fatalf("prepared = %+v", prepared)
	}
}

func TestPrepareMessagesResizesRequestCopyOnly(t *testing.T) {
	data := pngDataURLPayload(t, 2000, 2000)
	messages := []sdk.Message{{
		Role:  sdk.RoleUser,
		Parts: []sdk.ContentPart{{Type: sdk.ContentPartImage, MIMEType: "image/png", Data: data}},
	}}
	prepared, err := PrepareMessages(messages, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if messages[0].Parts[0].Data != data {
		t.Fatal("canonical source message was mutated")
	}
	if prepared[0].Parts[0].Data == data {
		t.Fatal("oversized image request copy was not transformed")
	}
	decoded, err := base64.StdEncoding.DecodeString(prepared[0].Parts[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 1600 || config.Height != 1600 {
		t.Fatalf("prepared dimensions = %dx%d, want 1600x1600", config.Width, config.Height)
	}
}
