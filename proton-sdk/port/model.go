package port

import (
	"context"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

// LanguageModel is the provider-neutral model boundary consumed by an agent loop.
// Providers own wire-format translation; callers only see Protonman SDK messages,
// tools, and stream events.
type LanguageModel interface {
	Provider() string
	ModelID() string
	Capabilities() domain.ModelCapabilities
	Stream(ctx context.Context, request domain.Request) (Stream, error)
}

// RequestPreparer optionally normalizes a request before the turn engine
// estimates context cost and applies compaction. Implementations must be
// idempotent for requests they have already prepared.
type RequestPreparer interface {
	PrepareRequest(context.Context, domain.Request) (domain.Request, error)
}

// MetadataModel is the canonical extension point for model metadata.
type MetadataModel interface {
	Metadata() domain.ModelMetadata
}

// TokenLimitsModel is the legacy token-limit extension point.
// Deprecated: implement MetadataModel instead.
type TokenLimitsModel interface {
	TokenLimits() domain.TokenLimits
}

// ContextWindowModel is the legacy context-window extension point.
// Deprecated: implement MetadataModel instead.
type ContextWindowModel interface {
	ContextWindow() int
}
