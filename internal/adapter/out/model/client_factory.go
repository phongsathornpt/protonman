package model

import (
	"net/http"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/agentidentity"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// clientConfig contains CLI-owned settings used to construct proton-sdk provider models.
type clientConfig struct {
	baseURL       string
	apiKey        string
	modelID       string
	sessionID     string
	clientName    string
	userAgent     string
	agentType     agentidentity.Type
	agentProfile  string
	httpClient    *http.Client
	vision        *bool
	tools         *bool
	contextWindow *int
	tokenLimits   *sdk.TokenLimits
	profile       *modelprofile.Resolved
}

// ClientOption configures provider model construction.
type ClientOption func(*clientConfig)

func WithSessionID(sessionID string) ClientOption {
	return func(c *clientConfig) { c.sessionID = sessionID }
}

func WithClientName(clientName string) ClientOption {
	return func(c *clientConfig) { c.clientName = clientName }
}

func WithUserAgent(userAgent string) ClientOption {
	return func(c *clientConfig) { c.userAgent = userAgent }
}

func WithAgentType(agentType agentidentity.Type) ClientOption {
	return func(c *clientConfig) { c.agentType = agentType }
}

func WithAgentProfile(profile string) ClientOption {
	return func(c *clientConfig) { c.agentProfile = strings.TrimSpace(profile) }
}

func WithVisionSupport(supported bool) ClientOption {
	return func(c *clientConfig) { c.vision = &supported }
}

func WithToolsSupport(supported bool) ClientOption {
	return func(c *clientConfig) { c.tools = &supported }
}

func WithContextWindow(tokens int) ClientOption {
	return func(c *clientConfig) {
		if tokens > 0 {
			c.contextWindow = &tokens
		}
	}
}

func WithRequestTimeout(timeout time.Duration) ClientOption {
	return func(c *clientConfig) {
		if timeout > 0 && c.httpClient != nil {
			c.httpClient.Timeout = timeout
		}
	}
}

func newClientConfig(baseURL, apiKey, modelID string) clientConfig {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultProtonmanEndpoint
	}
	return clientConfig{
		baseURL:    baseURL,
		apiKey:     apiKey,
		modelID:    strings.TrimSpace(modelID),
		clientName: "proton",
		userAgent:  buildinfo.UserAgent(),
		agentType:  agentidentity.TypeProtonman,
		httpClient: &http.Client{Timeout: runtimepolicy.ModelRequestTimeout},
	}
}

// NewProviderLanguageModel creates the proton-sdk language model for a configured provider protocol.
func NewProviderLanguageModel(
	providerName string,
	providerType string,
	baseURL string,
	apiKey string,
	modelID string,
	opts ...ClientOption,
) sdk.LanguageModel {
	protocol := ProviderProtocol(strings.ToLower(strings.TrimSpace(providerType)))
	if protocol == "" {
		if preset := MatchProviderPreset(providerName, baseURL); preset != nil {
			protocol = preset.Protocol
		}
	}
	baseURL = ResolveProviderBaseURLForProtocol(providerName, string(protocol), baseURL)
	builtinProfile := modelprofile.ResolveBuiltin(providerName, modelID, modelprofile.CatalogMetadata{})
	opts = append([]ClientOption{withResolvedModelProfile(builtinProfile)}, opts...)
	switch protocol {
	case ProviderProtocolAnthropic:
		return newSDKAnthropicLanguageModel(baseURL, apiKey, modelID, opts...)
	default:
		return newSDKOpenAILanguageModel(providerName, baseURL, apiKey, modelID, opts...)
	}
}
