package openai

import (
	"encoding/json"
	"testing"

	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

func TestResponsesRequestEncodesImageParts(t *testing.T) {
	provider := NewProvider(ProviderOptions{BaseURL: "https://example.test/v1"})
	languageModel := provider.Model("muse-spark-1.3-contributor", WithResponsesAPI())

	_, body, err := languageModel.encodeRequest(domain.Request{Messages: []domain.Message{{
		Role:    domain.RoleUser,
		Content: "inspect this screenshot",
		Parts: []domain.ContentPart{
			{Type: domain.ContentPartImage, MIMEType: "image/png", Data: "aW1hZ2U="},
			{Type: domain.ContentPartText, Text: "inspect this screenshot"},
		},
	}}})
	if err != nil {
		t.Fatalf("encodeRequest() error = %v", err)
	}

	var payload struct {
		Input []struct {
			Role    string `json:"role"`
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL string `json:"image_url"`
			} `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode request: %v\n%s", err, body)
	}
	if len(payload.Input) != 1 {
		t.Fatalf("input count = %d, want 1", len(payload.Input))
	}
	content := payload.Input[0].Content
	if len(content) != 2 {
		t.Fatalf("content count = %d, want 2: %s", len(content), body)
	}
	if content[0].Type != "input_image" || content[0].ImageURL != "data:image/png;base64,aW1hZ2U=" {
		t.Fatalf("image content = %+v", content[0])
	}
	if content[1].Type != "input_text" || content[1].Text != "inspect this screenshot" {
		t.Fatalf("text content = %+v", content[1])
	}
}

func TestChatRequestEncodesAttachedImageParts(t *testing.T) {
	provider := NewProvider(ProviderOptions{BaseURL: "https://example.test/v1"})
	languageModel := provider.Model("muse-spark-1.3-contributor")

	_, body, err := languageModel.encodeRequest(domain.Request{Messages: []domain.Message{{
		Role: domain.RoleUser,
		Parts: []domain.ContentPart{
			{Type: domain.ContentPartImage, MIMEType: "image/jpeg", Data: "aW1hZ2U="},
			{Type: domain.ContentPartText, Text: "describe the image"},
		},
	}}})
	if err != nil {
		t.Fatalf("encodeRequest() error = %v", err)
	}

	var payload struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode request: %v\n%s", err, body)
	}
	if payload.Model != "muse-spark-1.3-contributor" {
		t.Fatalf("model = %q", payload.Model)
	}
	if len(payload.Messages) != 1 || len(payload.Messages[0].Content) != 2 {
		t.Fatalf("messages = %#v", payload.Messages)
	}
	content := payload.Messages[0].Content
	if content[0].Type != "image_url" || content[0].ImageURL.URL != "data:image/jpeg;base64,aW1hZ2U=" {
		t.Fatalf("image content = %+v", content[0])
	}
	if content[1].Type != "text" || content[1].Text != "describe the image" {
		t.Fatalf("text content = %+v", content[1])
	}
}
