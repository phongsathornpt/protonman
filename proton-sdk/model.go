// Package protonsdk defines the provider-neutral model boundary used by Protonman agents.
package protonsdk

import (
	"context"
	"encoding/json"
)

// LanguageModel is the provider-neutral model boundary consumed by an agent loop.
// Providers own wire-format translation; callers only see Protonman SDK messages,
// tools, and stream events.
type LanguageModel interface {
	Provider() string
	ModelID() string
	Capabilities() ModelCapabilities
	Stream(ctx context.Context, request Request) (Stream, error)
}

// ModelCapabilities describes the effective runtime capabilities of a model instance.
// Provider models may publish protocol defaults; outer model-profile wrappers may narrow them.
type ModelCapabilities struct {
	Streaming        bool
	Tools            bool
	Vision           bool
	ProviderOptions  bool
	ToolResultErrors bool
	RawChunks        bool
}

// Satisfies reports whether the model can execute a request with the supplied
// provider-neutral requirements. Request requirements are derived before
// provider-specific lowering.
func (c ModelCapabilities) Satisfies(requirements RequestRequirements) bool {
	return (!requirements.Streaming || c.Streaming) &&
		(!requirements.Tools || c.Tools) &&
		(!requirements.Vision || c.Vision) &&
		(!requirements.ProviderOptions || c.ProviderOptions) &&
		(!requirements.RawChunks || c.RawChunks)
}

// RequestRequirements describes provider-neutral capabilities required to
// execute one canonical SDK request. The SDK exposes streaming as its model
// execution contract, so Streaming is always required for Request values.
type RequestRequirements struct {
	Streaming       bool
	Tools           bool
	Vision          bool
	ProviderOptions bool
	RawChunks       bool
}

// Requirements derives the effective model capabilities required by the
// request before provider-specific lowering.
func (r Request) Requirements() RequestRequirements {
	requirements := RequestRequirements{
		Streaming:       true,
		ProviderOptions: len(r.Options.ProviderOptions) > 0,
		RawChunks:       r.Options.IncludeRawChunks,
		Tools:           len(r.Tools) > 0 || r.Options.ToolChoice == ToolChoiceRequired,
	}

	for _, tool := range r.Tools {
		if len(tool.ProviderOptions) > 0 {
			requirements.ProviderOptions = true
		}
	}

	for _, message := range r.Messages {
		if message.Role == RoleTool || len(message.ToolCalls) > 0 {
			requirements.Tools = true
		}
		for _, part := range message.Parts {
			if part.Type == ContentPartImage {
				requirements.Vision = true
			}
		}
	}

	return requirements
}

// ModelMetadata groups optional provider-neutral metadata published by a model.
// New metadata should be added here instead of introducing another optional model interface.
type ModelMetadata struct {
	TokenLimits TokenLimits
}

// MetadataModel is the canonical extension point for model metadata.
type MetadataModel interface {
	Metadata() ModelMetadata
}

// ModelMetadataOf returns normalized model metadata. Legacy token-limit interfaces
// remain supported while callers and adapters migrate to MetadataModel.
func ModelMetadataOf(model LanguageModel) ModelMetadata {
	if model == nil {
		return ModelMetadata{}
	}
	if provider, ok := model.(MetadataModel); ok {
		metadata := provider.Metadata()
		metadata.TokenLimits = normalizeTokenLimits(metadata.TokenLimits)
		return metadata
	}
	return ModelMetadata{TokenLimits: legacyModelTokenLimits(model)}
}

// TokenLimits describes independently published model token constraints.
type TokenLimits struct {
	ContextWindow   int
	MaxInputTokens  int
	MaxOutputTokens int
}

// TokenLimitsModel is the legacy token-limit extension point.
// Deprecated: implement MetadataModel instead.
type TokenLimitsModel interface {
	TokenLimits() TokenLimits
}

// ContextWindowModel is the legacy context-window extension point.
// Deprecated: implement MetadataModel instead.
type ContextWindowModel interface {
	ContextWindow() int
}

// ModelTokenLimits returns all provider-neutral token limits published by a model.
func ModelTokenLimits(model LanguageModel) TokenLimits {
	return ModelMetadataOf(model).TokenLimits
}

// ModelContextWindow returns the model's authoritative context size when published.
func ModelContextWindow(model LanguageModel) int {
	return ModelTokenLimits(model).ContextWindow
}

func legacyModelTokenLimits(model LanguageModel) TokenLimits {
	if model == nil {
		return TokenLimits{}
	}
	if provider, ok := model.(TokenLimitsModel); ok {
		return normalizeTokenLimits(provider.TokenLimits())
	}
	return TokenLimits{ContextWindow: legacyContextWindow(model)}
}

func legacyContextWindow(model LanguageModel) int {
	provider, ok := model.(ContextWindowModel)
	if !ok {
		return 0
	}
	if tokens := provider.ContextWindow(); tokens > 0 {
		return tokens
	}
	return 0
}

func normalizeTokenLimits(limits TokenLimits) TokenLimits {
	if limits.ContextWindow < 0 {
		limits.ContextWindow = 0
	}
	if limits.MaxInputTokens < 0 {
		limits.MaxInputTokens = 0
	}
	if limits.MaxOutputTokens < 0 {
		limits.MaxOutputTokens = 0
	}
	return limits
}

type ProviderOptions map[string]json.RawMessage

// Clone returns a deep copy of provider options.
func (o ProviderOptions) Clone() ProviderOptions {
	if len(o) == 0 {
		return nil
	}
	cloned := make(ProviderOptions, len(o))
	for key, value := range o {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}

type ProviderMetadata map[string]json.RawMessage

// Clone returns a deep copy of provider metadata.
func (m ProviderMetadata) Clone() ProviderMetadata {
	return cloneProviderMetadata(m)
}

func cloneProviderMetadata(metadata ProviderMetadata) ProviderMetadata {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(ProviderMetadata, len(metadata))
	for key, value := range metadata {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}
