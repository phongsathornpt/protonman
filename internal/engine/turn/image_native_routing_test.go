package turn

import (
	"context"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type visionScriptedClient struct {
	*scriptedClient
}

func (*visionScriptedClient) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true, Tools: true, Vision: true}
}

func TestLoopRoutesAttachedImageAsNativeMultimodalInput(t *testing.T) {
	base := &scriptedClient{streams: []scriptedStreamSpec{{events: []sdk.Event{
		{Kind: sdk.EventTextDelta, Text: "done"},
		{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
	}}}}
	client := &visionScriptedClient{scriptedClient: base}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithSystemPromptSpec(prompt.Spec{}))

	imageData := encodedBlankPNG(t, 32, 32)
	_, err := loop.Run(context.Background(), []model.Message{{
		Role:    model.RoleUser,
		Content: "inspect this screenshot",
		Parts: []model.ContentPart{
			{Type: model.ContentPartImage, MIMEType: "image/png", Data: imageData},
			{Type: model.ContentPartText, Text: "inspect this screenshot"},
		},
	}}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(base.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(base.requests))
	}

	request := base.requests[0]
	if len(request.Messages) < 2 || request.Messages[0].Role != model.RoleSystem {
		t.Fatalf("request messages = %#v, want managed system prompt followed by user input", request.Messages)
	}
	systemPrompt := request.Messages[0].Content
	for _, want := range []string{
		"native multimodal input",
		"Do not use read, bash, Python, OCR, or another tool merely to inspect an image",
		"Never convert an attached image into text through Python",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, systemPrompt)
		}
	}

	foundImage := false
	for _, message := range request.Messages {
		for _, part := range message.Parts {
			if part.Type == model.ContentPartImage && part.MIMEType == "image/png" && part.Data != "" {
				foundImage = true
			}
		}
	}
	if !foundImage {
		t.Fatalf("native image content part was not preserved in model request: %#v", request.Messages)
	}
}

func TestTextOnlyTurnDoesNotInjectAttachedImagePolicy(t *testing.T) {
	base := &scriptedClient{streams: []scriptedStreamSpec{{events: []sdk.Event{
		{Kind: sdk.EventTextDelta, Text: "done"},
		{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
	}}}}
	client := &visionScriptedClient{scriptedClient: base}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithSystemPromptSpec(prompt.Spec{}))

	_, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect the repository"}}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(base.requests) != 1 || len(base.requests[0].Messages) == 0 {
		t.Fatalf("requests = %#v", base.requests)
	}
	if strings.Contains(base.requests[0].Messages[0].Content, "# Attached Images") {
		t.Fatalf("text-only prompt unexpectedly contains attached-image policy: %s", base.requests[0].Messages[0].Content)
	}
}
