package turn

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestEstimateRequestTokensDoesNotChargeImageBase64AsText(t *testing.T) {
	var encoded bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}

	// Keep a valid PNG header/config while making the transport representation
	// deliberately huge. DecodeConfig stops after image metadata, just as the
	// model context estimate should ignore irrelevant base64 payload size.
	encoded.Write(make([]byte, 1024*1024))
	data := base64.StdEncoding.EncodeToString(encoded.Bytes())

	request := sdk.Request{Messages: []sdk.Message{{
		Role: sdk.RoleUser,
		Parts: []sdk.ContentPart{{
			Type:     sdk.ContentPartImage,
			MIMEType: "image/png",
			Data:     data,
		}},
	}}}

	estimated, err := estimateRequestTokens(request)
	if err != nil {
		t.Fatalf("estimateRequestTokens() error = %v", err)
	}
	if estimated >= 20_000 {
		t.Fatalf("image estimate = %d tokens; base64 transport bytes appear to be charged as text", estimated)
	}
	if estimated < 4 {
		t.Fatalf("image estimate = %d tokens; expected 2x2 32px patches plus request framing", estimated)
	}
}

func TestEstimateImageTokensCapsPathologicalDimensions(t *testing.T) {
	// Invalid payloads use the active vision policy's conservative bounded
	// fallback rather than charging their entire encoded transport representation.
	data := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xff}, 1024*1024))
	want := modelprofile.DefaultVisionPolicy().FallbackTokens
	if got := estimateImageTokens(data); got != want {
		t.Fatalf("fallback image tokens = %d, want %d", got, want)
	}
}
