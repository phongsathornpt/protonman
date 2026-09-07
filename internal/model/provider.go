package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/modelprofile"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
)

type ProviderProtocol string

const (
	ProviderProtocolOpenAI    ProviderProtocol = "openai"
	ProviderProtocolAnthropic ProviderProtocol = "anthropic"

	// DefaultProtonmanName is the canonical provider label for Protonman.
	DefaultProtonmanName     = "protonman"
	DefaultProtonmanEndpoint = "https://protonman.dev/api/v1"

	// DefaultOpenCodeName is the canonical provider label for OpenCode Zen.
	DefaultOpenCodeName     = "opencode"
	DefaultOpenCodeEndpoint = "https://opencode.ai/zen/v1"

	// DefaultOllamaName is the canonical provider label for local Ollama.
	DefaultOllamaName     = "ollama"
	DefaultOllamaEndpoint = "http://localhost:11434/v1"

	// DefaultOpenAIName is the canonical provider label for OpenAI.
	DefaultOpenAIName     = "openai"
	DefaultOpenAIEndpoint = "https://api.openai.com/v1"

	// DefaultAnthropicName is the canonical provider label for Anthropic.
	DefaultAnthropicName     = "anthropic"
	DefaultAnthropicEndpoint = "https://api.anthropic.com"
)

// SupportedProviderPreset describes an out-of-the-box model provider preset.
type SupportedProviderPreset struct {
	ID             string
	Name           string
	Protocol       ProviderProtocol
	BaseURL        string
	EndpointHosts  []string
	RequiresKey    bool
	KeyPlaceholder string
	Description    string
}

// SupportedPresets lists available provider presets for discovery and quick setup.
var SupportedPresets = []SupportedProviderPreset{
	{
		ID:             DefaultOpenCodeName,
		Name:           "OpenCode (Free)",
		Protocol:       ProviderProtocolOpenAI,
		BaseURL:        DefaultOpenCodeEndpoint,
		EndpointHosts:  []string{"opencode.ai"},
		RequiresKey:    false,
		KeyPlaceholder: "API key (optional)…",
		Description:    "Free tier models, zero API key required",
	},
	{
		ID:             DefaultProtonmanName,
		Name:           "Protonman",
		Protocol:       ProviderProtocolOpenAI,
		BaseURL:        DefaultProtonmanEndpoint,
		EndpointHosts:  []string{"protonman.dev"},
		RequiresKey:    true,
		KeyPlaceholder: "plk_live_…",
		Description:    "High-speed AI models gateway (plk_...)",
	},
	{
		ID:             DefaultOllamaName,
		Name:           "Ollama (Local)",
		Protocol:       ProviderProtocolOpenAI,
		BaseURL:        DefaultOllamaEndpoint,
		EndpointHosts:  []string{"localhost", "127.0.0.1"},
		RequiresKey:    false,
		KeyPlaceholder: "API key (optional)…",
		Description:    "Local LLM inference, zero cloud cost",
	},
	{
		ID:             DefaultOpenAIName,
		Name:           "OpenAI Official",
		Protocol:       ProviderProtocolOpenAI,
		BaseURL:        DefaultOpenAIEndpoint,
		EndpointHosts:  []string{"api.openai.com"},
		RequiresKey:    true,
		KeyPlaceholder: "sk_…",
		Description:    "Direct OpenAI API access (sk-...)",
	},
	{
		ID:             DefaultAnthropicName,
		Name:           "Anthropic",
		Protocol:       ProviderProtocolAnthropic,
		BaseURL:        DefaultAnthropicEndpoint,
		EndpointHosts:  []string{"api.anthropic.com"},
		RequiresKey:    true,
		KeyPlaceholder: "sk-ant-…",
		Description:    "Direct Anthropic Messages API access",
	},
}

// LookupPreset returns the supported provider preset by ID or name, or nil if not matched.
func LookupPreset(idOrName string) *SupportedProviderPreset {
	clean := strings.ToLower(strings.TrimSpace(idOrName))
	for i := range SupportedPresets {
		p := &SupportedPresets[i]
		if strings.EqualFold(p.ID, clean) || strings.EqualFold(p.Name, clean) {
			return p
		}
	}
	return nil
}

// MatchProviderPreset resolves a known provider by configured name or endpoint.
func MatchProviderPreset(providerName, baseURL string) *SupportedProviderPreset {
	if preset := LookupPreset(providerName); preset != nil {
		return preset
	}
	endpoint := strings.ToLower(strings.TrimSpace(baseURL))
	if endpoint == "" {
		return nil
	}
	for i := range SupportedPresets {
		preset := &SupportedPresets[i]
		for _, host := range preset.EndpointHosts {
			if strings.Contains(endpoint, strings.ToLower(host)) {
				return preset
			}
		}
	}
	return nil
}

// IsProvider reports whether provider identity or endpoint maps to providerID.
func IsProvider(providerID, providerName, baseURL string) bool {
	preset := MatchProviderPreset(providerName, baseURL)
	return preset != nil && strings.EqualFold(preset.ID, providerID)
}

// ProviderHasUsableAuth reports whether a provider can be used with the supplied key.
func ProviderHasUsableAuth(providerName, baseURL, apiKey string) bool {
	if strings.TrimSpace(apiKey) != "" {
		return true
	}
	preset := MatchProviderPreset(providerName, baseURL)
	return preset != nil && !preset.RequiresKey
}

// ResolveProviderBaseURL returns the configured endpoint or the preset default
// for a known provider. Unknown providers retain the historical Protonman default.
func ResolveProviderBaseURL(providerName string, configuredURL string) string {
	return ResolveProviderBaseURLForProtocol(providerName, "", configuredURL)
}

// ResolveProviderBaseURLForProtocol resolves defaults using explicit protocol
// when a custom provider name does not match a built-in preset.
func ResolveProviderBaseURLForProtocol(providerName, providerType, configuredURL string) string {
	if baseURL := strings.TrimSpace(configuredURL); baseURL != "" {
		return strings.TrimRight(baseURL, "/")
	}
	if preset := LookupPreset(providerName); preset != nil {
		return preset.BaseURL
	}
	switch ProviderProtocol(strings.ToLower(strings.TrimSpace(providerType))) {
	case ProviderProtocolAnthropic:
		return DefaultAnthropicEndpoint
	case ProviderProtocolOpenAI:
		return DefaultOpenAIEndpoint
	default:
		return DefaultProtonmanEndpoint
	}
}

// RemoteModel describes a model discovered from an OpenAI or protonman endpoint.
type RemoteModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ContextWindow int      `json:"context_window,omitempty"`
	Provider      string   `json:"provider,omitempty"`
	Features      []string `json:"features,omitempty"`
	// ToolSupport and VisionSupport are tri-state capability metadata. Nil means
	// the catalog did not provide authoritative support information.
	ToolSupport        *bool                          `json:"tool_support,omitempty"`
	VisionSupport      *bool                          `json:"vision_support,omitempty"`
	ToolChoiceRequired *bool                          `json:"tool_choice_required,omitempty"`
	Reasoning          *modelprofile.CatalogReasoning `json:"reasoning,omitempty"`
}

// IsFreeModel reports whether a given model ID represents an OpenCode free-tier model.
func IsFreeModel(id string) bool {
	idLower := strings.ToLower(strings.TrimSpace(id))
	return strings.HasSuffix(idLower, "-free") || idLower == "big-pickle"
}

// FetchModelsOption configures provider model discovery.
type FetchModelsOption func(*http.Client)

// WithDiscoveryTimeout overrides the model-discovery HTTP timeout.
func WithDiscoveryTimeout(timeout time.Duration) FetchModelsOption {
	return func(client *http.Client) {
		if timeout > 0 {
			client.Timeout = timeout
		}
	}
}

// FetchProviderModels queries an OpenAI-compatible provider model endpoint.
func FetchProviderModels(ctx context.Context, baseURL string, apiKey string, options ...FetchModelsOption) ([]RemoteModel, error) {
	return FetchProviderModelsForProtocol(ctx, ProviderProtocolOpenAI, baseURL, apiKey, options...)
}

// FetchProviderModelsForProtocol queries the model catalog using protocol-specific authentication and response mapping.
func FetchProviderModelsForProtocol(ctx context.Context, protocol ProviderProtocol, baseURL string, apiKey string, options ...FetchModelsOption) ([]RemoteModel, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		if protocol == ProviderProtocolAnthropic {
			baseURL = DefaultAnthropicEndpoint
		} else {
			baseURL = DefaultProtonmanEndpoint
		}
	}

	client := &http.Client{Timeout: runtimepolicy.ModelDiscoveryTimeout}
	for _, option := range options {
		if option != nil {
			option(client)
		}
	}

	if protocol == ProviderProtocolAnthropic {
		return fetchAnthropicModels(ctx, client, baseURL, apiKey)
	}

	// 1. Try standard /models endpoint with Bearer auth (or without auth if apiKey is empty)
	modelsEndpoint := baseURL + "/models"
	models, err := fetchModelsFromURL(ctx, client, modelsEndpoint, apiKey)
	if err == nil && len(models) > 0 {
		return models, nil
	}

	// If 401 Unauthorized, return error directly to let user check their key
	if err != nil && strings.Contains(err.Error(), "401") {
		return nil, err
	}

	// 2. Try /public/models fallback (e.g. for protonman public catalog)
	publicEndpoint := baseURL + "/public/models"
	publicModels, publicErr := fetchModelsFromURL(ctx, client, publicEndpoint, "")
	if publicErr == nil && len(publicModels) > 0 {
		return publicModels, nil
	}

	if err != nil {
		return nil, err
	}
	return nil, errors.New("no models returned by endpoint")
}

func fetchModelsFromURL(ctx context.Context, client *http.Client, urlStr string, apiKey string) ([]RemoteModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (401): invalid or missing API key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Attempt parsing OpenAI format: {"data": [{"id": "model-id"}]}
	var openAIResp struct {
		Data []struct {
			ID             string   `json:"id"`
			Name           string   `json:"name"`
			ContextWindow  int      `json:"context_window"`
			ContextWindow2 int      `json:"contextWindow"`
			MaxInputTokens int      `json:"max_input_tokens"`
			Provider       string   `json:"provider"`
			Features       []string `json:"features"`
			Capabilities   struct {
				Tools              *bool `json:"tools"`
				Vision             *bool `json:"vision"`
				Reasoning          *bool `json:"reasoning"`
				ToolChoiceRequired *bool `json:"tool_choice_required"`
			} `json:"capabilities"`
			Reasoning *modelprofile.CatalogReasoning `json:"reasoning"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &openAIResp); err == nil && len(openAIResp.Data) > 0 {
		results := make([]RemoteModel, 0, len(openAIResp.Data))
		for _, item := range openAIResp.Data {
			name := item.Name
			if name == "" {
				name = item.ID
			}
			reasoning := item.Reasoning
			if reasoning == nil && item.Capabilities.Reasoning != nil {
				reasoning = &modelprofile.CatalogReasoning{Supported: item.Capabilities.Reasoning}
			}
			toolSupport := item.Capabilities.Tools
			visionSupport := item.Capabilities.Vision
			if toolSupport == nil && hasModelFeature(item.Features, "tools") {
				toolSupport = boolPointer(true)
			}
			if visionSupport == nil && hasModelFeature(item.Features, "vision") {
				visionSupport = boolPointer(true)
			}
			results = append(results, RemoteModel{
				ID:                 item.ID,
				Name:               name,
				ContextWindow:      firstPositiveInt(item.ContextWindow, item.ContextWindow2, item.MaxInputTokens),
				Provider:           item.Provider,
				Features:           item.Features,
				ToolSupport:        toolSupport,
				VisionSupport:      visionSupport,
				ToolChoiceRequired: item.Capabilities.ToolChoiceRequired,
				Reasoning:          modelprofile.NormalizeCatalogReasoning(reasoning),
			})
		}
		return results, nil
	}

	// Attempt parsing protonman format: {"models": [{"slug": "...", "name": "...", "contextWindow": 1000000}]}
	var protonmanResp struct {
		Models []struct {
			ID                         string   `json:"id"`
			Slug                       string   `json:"slug"`
			Name                       string   `json:"name"`
			ContextWindow              int      `json:"contextWindow"`
			Features                   []string `json:"features"`
			SupportsTools              *bool    `json:"supportsTools"`
			SupportsVision             *bool    `json:"supportsVision"`
			SupportsToolChoiceRequired *bool    `json:"supportsToolChoiceRequired"`
			Capabilities               struct {
				Tools              *bool `json:"tools"`
				Vision             *bool `json:"vision"`
				Reasoning          *bool `json:"reasoning"`
				ToolChoiceRequired *bool `json:"tool_choice_required"`
			} `json:"capabilities"`
			Reasoning *modelprofile.CatalogReasoning `json:"reasoning"`
			Provider  struct {
				Name string `json:"name"`
			} `json:"provider"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &protonmanResp); err == nil && len(protonmanResp.Models) > 0 {
		results := make([]RemoteModel, 0, len(protonmanResp.Models))
		for _, item := range protonmanResp.Models {
			id := item.Slug
			if id == "" {
				id = item.ID
			}
			toolSupport := firstKnownBool(item.Capabilities.Tools, item.SupportsTools)
			visionSupport := firstKnownBool(item.Capabilities.Vision, item.SupportsVision)
			requiredToolChoice := firstKnownBool(item.Capabilities.ToolChoiceRequired, item.SupportsToolChoiceRequired)
			if toolSupport == nil && hasModelFeature(item.Features, "tools") {
				toolSupport = boolPointer(true)
			}
			if visionSupport == nil && hasModelFeature(item.Features, "vision") {
				visionSupport = boolPointer(true)
			}
			reasoning := item.Reasoning
			if reasoning == nil && item.Capabilities.Reasoning != nil {
				reasoning = &modelprofile.CatalogReasoning{Supported: item.Capabilities.Reasoning}
			}
			results = append(results, RemoteModel{
				ID:                 id,
				Name:               item.Name,
				ContextWindow:      item.ContextWindow,
				Provider:           item.Provider.Name,
				Features:           item.Features,
				ToolSupport:        toolSupport,
				VisionSupport:      visionSupport,
				ToolChoiceRequired: requiredToolChoice,
				Reasoning:          modelprofile.NormalizeCatalogReasoning(reasoning),
			})
		}
		return results, nil
	}

	return nil, errors.New("unrecognized models response format")
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func boolPointer(value bool) *bool { return &value }

func firstKnownBool(values ...*bool) *bool {
	for _, value := range values {
		if value != nil {
			copy := *value
			return &copy
		}
	}
	return nil
}

func hasModelFeature(features []string, want string) bool {
	for _, feature := range features {
		if strings.EqualFold(strings.TrimSpace(feature), want) {
			return true
		}
	}
	return false
}

func fetchAnthropicModels(ctx context.Context, client *http.Client, baseURL string, apiKey string) ([]RemoteModel, error) {
	endpoint := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/models"
	} else {
		endpoint += "/v1/models"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create anthropic models request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch anthropic models: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read anthropic models response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed (401): invalid or missing API key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Data []struct {
			ID             string `json:"id"`
			DisplayName    string `json:"display_name"`
			MaxInputTokens int    `json:"max_input_tokens"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode anthropic models response: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, errors.New("no models returned by endpoint")
	}
	models := make([]RemoteModel, 0, len(payload.Data))
	for _, item := range payload.Data {
		name := item.DisplayName
		if strings.TrimSpace(name) == "" {
			name = item.ID
		}
		models = append(models, RemoteModel{ID: item.ID, Name: name, ContextWindow: item.MaxInputTokens, Provider: DefaultAnthropicName})
	}
	return models, nil
}
