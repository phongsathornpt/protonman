package turn

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
)

func encodedBlankPNG(t *testing.T, width, height int) string {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func TestEstimateImageTokensUsesResolvedVisionScheme(t *testing.T) {
	data := encodedBlankPNG(t, 1000, 1000)

	patch := estimateImageTokensWithPolicy(data, modelprofile.DefaultVisionPolicy())
	if patch != 1024 {
		t.Fatalf("patch tokens = %d, want 1024", patch)
	}

	anthropic := estimateImageTokensWithPolicy(data, modelprofile.VisionPolicy{
		MaxDimension: 1568, MaxPatches: 1160, PatchSize: 32,
		TokenScheme: modelprofile.VisionTokenAnthropicPixels, FallbackTokens: 1600,
	})
	if anthropic != 1334 {
		t.Fatalf("anthropic tokens = %d, want 1334", anthropic)
	}

	gemini := estimateImageTokensWithPolicy(data, modelprofile.VisionPolicy{
		MaxDimension: 6000, MaxPatches: 10000, PatchSize: 32,
		TokenScheme: modelprofile.VisionTokenGeminiTiles, FallbackTokens: 2064,
	})
	if gemini != 1032 {
		t.Fatalf("gemini tokens = %d, want 1032", gemini)
	}
}

func TestEstimateGeminiSmallImageUsesSingleFixedCharge(t *testing.T) {
	data := encodedBlankPNG(t, 384, 384)
	got := estimateImageTokensWithPolicy(data, modelprofile.VisionPolicy{
		TokenScheme: modelprofile.VisionTokenGeminiTiles,
	})
	if got != 258 {
		t.Fatalf("gemini small image tokens = %d, want 258", got)
	}
}

func TestEstimateImageTokensUsesPolicyFallbackForInvalidPayload(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("not an image"))
	got := estimateImageTokensWithPolicy(data, modelprofile.VisionPolicy{
		TokenScheme: modelprofile.VisionTokenAnthropicPixels,
		FallbackTokens: 1600,
	})
	if got != 1600 {
		t.Fatalf("fallback tokens = %d, want 1600", got)
	}
}
