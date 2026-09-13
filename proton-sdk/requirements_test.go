package protonsdk

import "testing"

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
