package domain_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/port"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/usecase"
)

type languageModelStub struct {
	provider string
	modelID  string
}

func (m languageModelStub) Provider() string { return m.provider }
func (m languageModelStub) ModelID() string  { return m.modelID }
func (m languageModelStub) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true}
}
func (m languageModelStub) Stream(context.Context, domain.Request) (port.Stream, error) {
	return emptyStream{}, nil
}

type emptyStream struct{}

func (emptyStream) Next(context.Context) (domain.Event, error) { return domain.Event{}, io.EOF }
func (emptyStream) Close() error                               { return nil }

func TestLanguageModelContract(t *testing.T) {
	var model port.LanguageModel = languageModelStub{provider: "openai", modelID: "test-model"}
	if model.Provider() != "openai" {
		t.Fatalf("Provider() = %q", model.Provider())
	}
	if model.ModelID() != "test-model" {
		t.Fatalf("ModelID() = %q", model.ModelID())
	}
	if caps := model.Capabilities(); !caps.Streaming {
		t.Fatalf("Capabilities() = %#v, want streaming", caps)
	}
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

type tokenLimitsTestModel struct {
	port.LanguageModel
	limits domain.TokenLimits
}

func (m tokenLimitsTestModel) TokenLimits() domain.TokenLimits { return m.limits }

func TestModelTokenLimitsUsesRichMetadata(t *testing.T) {
	got := usecase.ModelTokenLimits(tokenLimitsTestModel{limits: domain.TokenLimits{ContextWindow: 100, MaxInputTokens: 80, MaxOutputTokens: 20}})
	if got.ContextWindow != 100 || got.MaxInputTokens != 80 || got.MaxOutputTokens != 20 {
		t.Fatalf("ModelTokenLimits() = %+v", got)
	}
}

func TestRequestRequirementsDerivesCanonicalNeeds(t *testing.T) {
	tests := []struct {
		name string
		req  domain.Request
		want domain.RequestRequirements
	}{
		{
			name: "text stream",
			req:  domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hello"}}},
			want: domain.RequestRequirements{Streaming: true},
		},
		{
			name: "vision input",
			req:  domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Parts: []domain.ContentPart{{Type: domain.ContentPartImage, MIMEType: "image/png", Data: "abc"}}}}},
			want: domain.RequestRequirements{Streaming: true, Vision: true},
		},
		{
			name: "tool definitions",
			req: domain.Request{
				Messages: []domain.Message{{Role: domain.RoleUser, Content: "inspect"}},
				Tools:    []domain.Tool{{Name: "read", Description: "read a file"}},
			},
			want: domain.RequestRequirements{Streaming: true, Tools: true},
		},
		{
			name: "tool history",
			req:  domain.Request{Messages: []domain.Message{{Role: domain.RoleTool, ToolCallID: "call_1", ToolName: "read", Content: "done"}}},
			want: domain.RequestRequirements{Streaming: true, Tools: true},
		},
		{
			name: "provider options and raw chunks",
			req: domain.Request{
				Messages: []domain.Message{{Role: domain.RoleUser, Content: "hello"}},
				Options: domain.ModelOptions{
					ProviderOptions:  domain.ProviderOptions{"openai": []byte(`{"service_tier":"flex"}`)},
					IncludeRawChunks: true,
				},
			},
			want: domain.RequestRequirements{Streaming: true, ProviderOptions: true, RawChunks: true},
		},
		{
			name: "tool provider options",
			req: domain.Request{
				Messages: []domain.Message{{Role: domain.RoleUser, Content: "inspect"}},
				Tools: []domain.Tool{{
					Name:            "read",
					Description:     "read a file",
					ProviderOptions: domain.ProviderOptions{"anthropic": []byte(`{"cache_control":{"type":"ephemeral"}}`)},
				}},
			},
			want: domain.RequestRequirements{Streaming: true, Tools: true, ProviderOptions: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.req.Requirements(); got != tt.want {
				t.Fatalf("Requirements() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestModelCapabilitiesSatisfiesRequestRequirements(t *testing.T) {
	all := domain.ModelCapabilities{
		Streaming:       true,
		Tools:           true,
		Vision:          true,
		ProviderOptions: true,
		RawChunks:       true,
	}
	requirements := domain.RequestRequirements{
		Streaming:       true,
		Tools:           true,
		Vision:          true,
		ProviderOptions: true,
		RawChunks:       true,
	}
	if !all.Satisfies(requirements) {
		t.Fatal("full capabilities should satisfy full requirements")
	}

	checks := []struct {
		name string
		caps domain.ModelCapabilities
	}{
		{name: "streaming", caps: domain.ModelCapabilities{Tools: true, Vision: true, ProviderOptions: true, RawChunks: true}},
		{name: "tools", caps: domain.ModelCapabilities{Streaming: true, Vision: true, ProviderOptions: true, RawChunks: true}},
		{name: "vision", caps: domain.ModelCapabilities{Streaming: true, Tools: true, ProviderOptions: true, RawChunks: true}},
		{name: "provider options", caps: domain.ModelCapabilities{Streaming: true, Tools: true, Vision: true, RawChunks: true}},
		{name: "raw chunks", caps: domain.ModelCapabilities{Streaming: true, Tools: true, Vision: true, ProviderOptions: true}},
	}
	for _, tt := range checks {
		t.Run(tt.name, func(t *testing.T) {
			if tt.caps.Satisfies(requirements) {
				t.Fatalf("capabilities %#v unexpectedly satisfy %#v", tt.caps, requirements)
			}
		})
	}
}

func TestProviderOptionsClone(t *testing.T) {
	original := domain.ProviderOptions{
		"openai":    json.RawMessage(`{"parallel_tool_calls":true}`),
		"anthropic": json.RawMessage(`{"cache_control":{"type":"ephemeral"}}`),
	}
	cloned := original.Clone()
	if len(cloned) != 2 {
		t.Fatalf("cloned len = %d, want 2", len(cloned))
	}
	cloned["openai"][0] = '['
	if string(original["openai"]) != `{"parallel_tool_calls":true}` {
		t.Fatalf("original provider options mutated: %s", original["openai"])
	}
}

func TestProviderMetadataClone(t *testing.T) {
	original := domain.ProviderMetadata{
		"openai": json.RawMessage(`{"request_id":"req_1"}`),
	}
	cloned := original.Clone()
	if len(cloned) != 1 {
		t.Fatalf("cloned len = %d, want 1", len(cloned))
	}
	cloned["openai"][0] = '['
	if string(original["openai"]) != `{"request_id":"req_1"}` {
		t.Fatalf("original provider metadata mutated: %s", original["openai"])
	}
}

func TestProviderOptionsCloneEmpty(t *testing.T) {
	var empty domain.ProviderOptions
	if empty.Clone() != nil {
		t.Fatal("empty ProviderOptions.Clone() should return nil")
	}
	var emptyMeta domain.ProviderMetadata
	if emptyMeta.Clone() != nil {
		t.Fatal("empty ProviderMetadata.Clone() should return nil")
	}
}
