package turn

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func imagePayloadWithTransportPadding(t *testing.T, padding int) string {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 64, 64))); err != nil {
		t.Fatal(err)
	}
	encoded.Write(bytes.Repeat([]byte{0}, padding))
	return base64.StdEncoding.EncodeToString(encoded.Bytes())
}

func TestCompactionIgnoresImageTransportPayloadSize(t *testing.T) {
	policy := modelprofile.CompactionPolicy{
		SoftThresholdRatio:       0.30,
		MediumThresholdRatio:     0.50,
		AggressiveThresholdRatio: 0.70,
		EmergencyThresholdRatio:  0.90,
		TargetRatio:              0.25,
		MinRecentMessages:        2,
	}
	vision := modelprofile.DefaultVisionPolicy()
	limits := sdk.TokenLimits{MaxInputTokens: 4_000}

	build := func(imageData string) sdk.Request {
		messages := []sdk.Message{{Role: sdk.RoleSystem, Content: "system"}}
		for i := 0; i < 10; i++ {
			messages = append(messages, sdk.Message{Role: sdk.RoleUser, Content: strings.Repeat("historical context ", 60)})
		}
		messages = append(messages,
			sdk.Message{Role: sdk.RoleUser, Content: "inspect this", Parts: []sdk.ContentPart{{Type: sdk.ContentPartImage, MIMEType: "image/png", Data: imageData}}},
			sdk.Message{Role: sdk.RoleAssistant, Content: "CURRENT-ASSISTANT"},
		)
		return sdk.Request{Messages: messages}
	}

	small := build(imagePayloadWithTransportPadding(t, 0))
	huge := build(imagePayloadWithTransportPadding(t, 1024*1024))

	smallResult, smallDecision, err := compactRequestToModelBudgetWithVisionPolicy(small, limits, policy, vision)
	if err != nil {
		t.Fatal(err)
	}
	hugeResult, hugeDecision, err := compactRequestToModelBudgetWithVisionPolicy(huge, limits, policy, vision)
	if err != nil {
		t.Fatal(err)
	}
	if !smallDecision.Required() || !hugeDecision.Required() {
		t.Fatalf("expected both requests to compact: small=%+v huge=%+v", smallDecision, hugeDecision)
	}
	if len(smallResult.Messages) != len(hugeResult.Messages) {
		t.Fatalf("retained message count differs by transport size: small=%d huge=%d", len(smallResult.Messages), len(hugeResult.Messages))
	}
	for i := range smallResult.Messages {
		if smallResult.Messages[i].Role != hugeResult.Messages[i].Role || smallResult.Messages[i].Content != hugeResult.Messages[i].Content {
			t.Fatalf("retained history differs at %d: small=%+v huge=%+v", i, smallResult.Messages[i], hugeResult.Messages[i])
		}
	}
	last := hugeResult.Messages[len(hugeResult.Messages)-1]
	if last.Content != "CURRENT-ASSISTANT" {
		t.Fatalf("latest turn was not preserved: %+v", last)
	}
}
