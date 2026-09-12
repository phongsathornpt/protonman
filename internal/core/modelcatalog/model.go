// Package modelcatalog owns provider-neutral discovered model metadata.
package modelcatalog

import (
	"context"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
)

// RemoteModel describes a model discovered from a provider catalog.
type RemoteModel struct {
	ID                 string                          `json:"id"`
	Name               string                          `json:"name"`
	ContextWindow      int                             `json:"context_window,omitempty"`
	MaxInputTokens     int                             `json:"max_input_tokens,omitempty"`
	MaxOutputTokens    int                             `json:"max_output_tokens,omitempty"`
	Provider           string                          `json:"provider,omitempty"`
	Features           []string                        `json:"features,omitempty"`
	ToolSupport        *bool                           `json:"tool_support,omitempty"`
	VisionSupport      *bool                           `json:"vision_support,omitempty"`
	ToolChoiceRequired *bool                           `json:"tool_choice_required,omitempty"`
	Reasoning          *modelprofile.CatalogReasoning `json:"reasoning,omitempty"`
}
// DiscoveryRequest describes one provider catalog lookup.
type DiscoveryRequest struct {
	ProviderName string
	ProviderType string
	BaseURL      string
	APIKey       string
	Timeout      time.Duration
}

// Discovery is the provider-catalog outbound port.
type Discovery interface {
	Discover(context.Context, DiscoveryRequest) ([]RemoteModel, error)
}

// ProfileMetadata projects catalog metadata into the model-profile domain.
func (m RemoteModel) ProfileMetadata() modelprofile.CatalogMetadata {
	return modelprofile.CatalogMetadata{
		Tools:              m.ToolSupport,
		Vision:             m.VisionSupport,
		ToolChoiceRequired: m.ToolChoiceRequired,
		ContextWindow:      m.ContextWindow,
		MaxInputTokens:     m.MaxInputTokens,
		MaxOutputTokens:    m.MaxOutputTokens,
		Reasoning:          modelprofile.NormalizeCatalogReasoning(m.Reasoning),
	}
}
