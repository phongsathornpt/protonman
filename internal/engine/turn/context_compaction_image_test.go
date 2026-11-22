package turn

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
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
	limits := domain.TokenLimits{MaxInputTokens: 4_000}

	build := func(imageData string) domain.Request {
		messages := []domain.Message{{Role: domain.RoleSystem, Content: "system"}}
		for i := 0; i < 10; i++ {
			messages = append(messages, domain.Message{Role: domain.RoleUser, Content: strings.Repeat("historical context ", 60)})
		}
		messages = append(messages,
			domain.Message{Role: domain.RoleUser, Content: "inspect this", Parts: []domain.ContentPart{{Type: domain.ContentPartImage, MIMEType: "image/png", Data: imageData}}},
			domain.Message{Role: domain.RoleAssistant, Content: "CURRENT-ASSISTANT"},
		)
		return domain.Request{Messages: messages}
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
