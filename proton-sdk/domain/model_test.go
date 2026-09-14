package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func TestModelCapabilitiesSatisfies(t *testing.T) {
	caps := domain.ModelCapabilities{
		Streaming:        true,
		Tools:            true,
		Vision:           true,
		ProviderOptions:  true,
		ToolResultErrors: true,
		RawChunks:        true,
	}
	req := domain.RequestRequirements{
		Streaming:       true,
		Tools:           true,
		Vision:          true,
		ProviderOptions: true,
		RawChunks:       true,
	}
	if !caps.Satisfies(req) {
		t.Fatal("caps should satisfy full requirements")
	}

	missingStreaming := caps
	missingStreaming.Streaming = false
	if missingStreaming.Satisfies(req) {
		t.Fatal("missing streaming should not satisfy requirements")
	}

	missingTools := caps
	missingTools.Tools = false
	if missingTools.Satisfies(req) {
		t.Fatal("missing tools should not satisfy requirements")
	}

	missingVision := caps
	missingVision.Vision = false
	if missingVision.Satisfies(req) {
		t.Fatal("missing vision should not satisfy requirements")
	}

	missingOptions := caps
	missingOptions.ProviderOptions = false
	if missingOptions.Satisfies(req) {
		t.Fatal("missing provider options should not satisfy requirements")
	}

	missingRaw := caps
	missingRaw.RawChunks = false
	if missingRaw.Satisfies(req) {
		t.Fatal("missing raw chunks should not satisfy requirements")
	}
}

func TestRequestRequirements(t *testing.T) {
	req := domain.Request{
		Messages: []domain.Message{
			{Role: domain.RoleUser, Content: "hi"},
			{Role: domain.RoleUser, Parts: []domain.ContentPart{{Type: domain.ContentPartImage, Data: "img"}}},
			{Role: domain.RoleTool, ToolCallID: "call_1"},
		},
		Tools: []domain.Tool{
			{Name: "read", Description: "read file", ProviderOptions: domain.ProviderOptions{"opt": json.RawMessage(`{}`)}},
		},
		Options: domain.ModelOptions{
			ProviderOptions:  domain.ProviderOptions{"openai": json.RawMessage(`{}`)},
			IncludeRawChunks: true,
			ToolChoice:       domain.ToolChoiceRequired,
		},
	}
	needs := req.Requirements()
	if !needs.Streaming || !needs.Tools || !needs.Vision || !needs.ProviderOptions || !needs.RawChunks {
		t.Fatalf("unexpected requirements: %+v", needs)
	}
}

func TestNormalizeTokenLimits(t *testing.T) {
	limits := domain.TokenLimits{
		ContextWindow:   -10,
		MaxInputTokens:  -5,
		MaxOutputTokens: -1,
	}
	normalized := domain.NormalizeTokenLimits(limits)
	if normalized.ContextWindow != 0 || normalized.MaxInputTokens != 0 || normalized.MaxOutputTokens != 0 {
		t.Fatalf("expected all zeros, got %+v", normalized)
	}
}

func TestProviderOptionsAndMetadataClone(t *testing.T) {
	opts := domain.ProviderOptions{"k": json.RawMessage(`{"a":1}`)}
	clonedOpts := opts.Clone()
	clonedOpts["k"][0] = '['
	if string(opts["k"]) != `{"a":1}` {
		t.Fatal("opts was mutated")
	}

	var nilOpts domain.ProviderOptions
	if nilOpts.Clone() != nil {
		t.Fatal("nil opts clone should be nil")
	}

	meta := domain.ProviderMetadata{"m": json.RawMessage(`{"b":2}`)}
	clonedMeta := meta.Clone()
	clonedMeta["m"][0] = '['
	if string(meta["m"]) != `{"b":2}` {
		t.Fatal("meta was mutated")
	}

	var nilMeta domain.ProviderMetadata
	if nilMeta.Clone() != nil {
		t.Fatal("nil meta clone should be nil")
	}
}
