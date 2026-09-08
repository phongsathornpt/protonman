package protonsdk

import "context"

// LanguageModel is the provider-neutral model boundary consumed by an agent loop.
// Providers own wire-format translation; callers only see Protonman SDK messages,
// tools, and stream events.
type LanguageModel interface {
	Provider() string
	ModelID() string
	Capabilities() ModelCapabilities
	Stream(ctx context.Context, request Request) (Stream, error)
}
