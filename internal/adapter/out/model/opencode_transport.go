package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
)

// openCodeTransport describes the wire endpoint selected by a direct OpenCode
// Zen model. The SDK remains provider-neutral; this routing policy belongs in
// the CLI-owned model adapter.
type openCodeTransport uint8

const (
	openCodeTransportChat openCodeTransport = iota
	openCodeTransportResponses
	openCodeTransportMessages
	openCodeTransportUnsupported
)

func resolveOpenCodeTransport(providerName, baseURL, modelID string) openCodeTransport {
	if !isOpenCodeZenRoute(providerName, baseURL) {
		return openCodeTransportChat
	}

	id := modelIDLeafForTransport(modelID)
	switch {
	case hasTransportPrefix(id, "gpt-", "grok-", "muse-spark"):
		return openCodeTransportResponses
	case hasTransportPrefix(id, "claude-", "qwen"):
		return openCodeTransportMessages
	case hasTransportPrefix(
		id,
		"deepseek-",
		"glm-",
		"kimi-",
		"minimax-",
		"big-pickle",
		"space-bunny-",
		"mimo-",
		"ling-",
		"nemotron-",
	):
		return openCodeTransportChat
	default:
		return openCodeTransportUnsupported
	}
}

func isOpenCodeZenRoute(providerName, baseURL string) bool {
	endpoint := strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if strings.HasSuffix(endpoint, "/zen/v1") || strings.HasSuffix(endpoint, "/zen/go/v1") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(providerName), DefaultOpenCodeName) &&
		strings.Contains(endpoint, "opencode.ai/zen") {
		return true
	}
	return false
}

// IsOpenCodeRoute reports whether a configured route belongs to an OpenCode
// provider, including the separate keyless and authenticated endpoints.
func IsOpenCodeRoute(providerName, baseURL string) bool {
	if IsOpenCodeInferenceEndpoint(baseURL) || IsOpenCodeZenEndpoint(baseURL) || IsOpenCodeGoEndpoint(baseURL) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(providerName), DefaultOpenCodeName) &&
		strings.Contains(strings.ToLower(strings.TrimSpace(baseURL)), "opencode.ai/")
}

func modelIDLeafForTransport(modelID string) string {
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	if slash := strings.LastIndexByte(modelID, '/'); slash >= 0 && slash+1 < len(modelID) {
		return modelID[slash+1:]
	}
	return modelID
}

func hasTransportPrefix(modelID string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(modelID, prefix) {
			return true
		}
	}
	return false
}

func filterOpenCodeCatalog(models []RemoteModel) []RemoteModel {
	visible := make([]RemoteModel, 0, len(models))
	for _, candidate := range models {
		if resolveOpenCodeTransport(DefaultOpenCodeName, OpenCodeZenEndpoint, candidate.ID) == openCodeTransportUnsupported {
			continue
		}
		visible = append(visible, candidate)
	}
	return visible
}

// unsupportedOpenCodeModel preserves a useful construction-time error for a
// stale saved selection instead of allowing a wrong endpoint to return an
// opaque 404 on the first turn.
type unsupportedOpenCodeModel struct {
	provider string
	modelID  string
	endpoint string
}

func (m *unsupportedOpenCodeModel) Provider() string {
	return m.provider
}

func (m *unsupportedOpenCodeModel) ModelID() string {
	return m.modelID
}

func (m *unsupportedOpenCodeModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true}
}

func (m *unsupportedOpenCodeModel) Stream(context.Context, domain.Request) (port.Stream, error) {
	return nil, fmt.Errorf("unsupported OpenCode model transport for %q at %s", m.modelID, m.endpoint)
}

var _ port.LanguageModel = (*unsupportedOpenCodeModel)(nil)
