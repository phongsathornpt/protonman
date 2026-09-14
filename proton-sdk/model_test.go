package protonsdk

import (
	"context"
	"encoding/json"
	"io"
	"testing"
)

type languageModelStub struct {
	provider string
	modelID  string
}

func (m languageModelStub) Provider() string { return m.provider }
func (m languageModelStub) ModelID() string  { return m.modelID }
func (m languageModelStub) Capabilities() ModelCapabilities {
	return ModelCapabilities{Streaming: true}
}
func (m languageModelStub) Stream(context.Context, Request) (Stream, error) {
	return emptyStream{}, nil
}

type emptyStream struct{}

func (emptyStream) Next(context.Context) (Event, error) { return Event{}, io.EOF }
func (emptyStream) Close() error                        { return nil }

func TestLanguageModelContract(t *testing.T) {
	var model LanguageModel = languageModelStub{provider: "openai", modelID: "test-model"}
	if model.Provider() != "openai" {
		t.Fatalf("Provider() = %q", model.Provider())
	}
	if model.ModelID() != "test-model" {
		t.Fatalf("ModelID() = %q", model.ModelID())
	}
	if caps := model.Capabilities(); !caps.Streaming {
		t.Fatalf("Capabilities() = %#v, want streaming", caps)
	}
	stream, err := model.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

type tokenLimitsTestModel struct {
	LanguageModel
	limits TokenLimits
}

func (m tokenLimitsTestModel) TokenLimits() TokenLimits { return m.limits }

func TestModelTokenLimitsUsesRichMetadata(t *testing.T) {
	got := ModelTokenLimits(tokenLimitsTestModel{limits: TokenLimits{ContextWindow: 100, MaxInputTokens: 80, MaxOutputTokens: 20}})
	if got.ContextWindow != 100 || got.MaxInputTokens != 80 || got.MaxOutputTokens != 20 {
		t.Fatalf("ModelTokenLimits() = %+v", got)
	}
}

func TestRequestRequirementsDerivesCanonicalNeeds(t *testing.T) {
	tests := []struct {
		name string
		req  Request
		want RequestRequirements
	}{
		{
			name: "text stream",
			req:  Request{Messages: []Message{{Role: RoleUser, Content: "hello"}}},
			want: RequestRequirements{Streaming: true},
		},
		{
			name: "vision input",
			req:  Request{Messages: []Message{{Role: RoleUser, Parts: []ContentPart{{Type: ContentPartImage, MIMEType: "image/png", Data: "abc"}}}}},
			want: RequestRequirements{Streaming: true, Vision: true},
		},
		{
			name: "tool definitions",
			req: Request{
				Messages: []Message{{Role: RoleUser, Content: "inspect"}},
				Tools:    []Tool{{Name: "read", Description: "read a file"}},
			},
			want: RequestRequirements{Streaming: true, Tools: true},
		},
		{
			name: "tool history",
			req:  Request{Messages: []Message{{Role: RoleTool, ToolCallID: "call_1", ToolName: "read", Content: "done"}}},
			want: RequestRequirements{Streaming: true, Tools: true},
		},
		{
			name: "provider options and raw chunks",
			req: Request{
				Messages: []Message{{Role: RoleUser, Content: "hello"}},
				Options: ModelOptions{
					ProviderOptions:  ProviderOptions{"openai": []byte(`{"service_tier":"flex"}`)},
					IncludeRawChunks: true,
				},
			},
			want: RequestRequirements{Streaming: true, ProviderOptions: true, RawChunks: true},
		},
		{
			name: "tool provider options",
			req: Request{
				Messages: []Message{{Role: RoleUser, Content: "inspect"}},
				Tools: []Tool{{
					Name:            "read",
					Description:     "read a file",
					ProviderOptions: ProviderOptions{"anthropic": []byte(`{"cache_control":{"type":"ephemeral"}}`)},
				}},
			},
			want: RequestRequirements{Streaming: true, Tools: true, ProviderOptions: true},
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
	all := ModelCapabilities{
		Streaming:       true,
		Tools:           true,
		Vision:          true,
		ProviderOptions: true,
		RawChunks:       true,
	}
	requirements := RequestRequirements{
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
		caps ModelCapabilities
	}{
		{name: "streaming", caps: ModelCapabilities{Tools: true, Vision: true, ProviderOptions: true, RawChunks: true}},
		{name: "tools", caps: ModelCapabilities{Streaming: true, Vision: true, ProviderOptions: true, RawChunks: true}},
		{name: "vision", caps: ModelCapabilities{Streaming: true, Tools: true, ProviderOptions: true, RawChunks: true}},
		{name: "provider options", caps: ModelCapabilities{Streaming: true, Tools: true, Vision: true, RawChunks: true}},
		{name: "raw chunks", caps: ModelCapabilities{Streaming: true, Tools: true, Vision: true, ProviderOptions: true}},
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
	original := ProviderOptions{
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
	original := ProviderMetadata{
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
	var empty ProviderOptions
	if empty.Clone() != nil {
		t.Fatal("empty ProviderOptions.Clone() should return nil")
	}
	var emptyMeta ProviderMetadata
	if emptyMeta.Clone() != nil {
		t.Fatal("empty ProviderMetadata.Clone() should return nil")
	}
}
