package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"io"
	"net/http"
	"strings"
)

type ProviderProtocol string

const (
	ProviderProtocolOpenAI ProviderProtocol = "openai"

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
)

// SupportedProviderPreset describes an out-of-the-box model provider preset.
type SupportedProviderPreset struct {
	ID            string
	Name          string
	Protocol      ProviderProtocol
	BaseURL       string
	EndpointHosts []string
	RequiresKey   bool
	Description   string
}

// SupportedPresets lists available provider presets for discovery and quick setup.
var SupportedPresets = []SupportedProviderPreset{
	{
		ID:            DefaultOpenCodeName,
		Name:          "OpenCode (Free)",
		Protocol:      ProviderProtocolOpenAI,
		BaseURL:       DefaultOpenCodeEndpoint,
		EndpointHosts: []string{"opencode.ai"},
		RequiresKey:   false,
		Description:   "Free tier models, zero API key required",
	},
	{
		ID:            DefaultProtonmanName,
		Name:          "Protonman",
		Protocol:      ProviderProtocolOpenAI,
		BaseURL:       DefaultProtonmanEndpoint,
		EndpointHosts: []string{"protonman.dev"},
		RequiresKey:   true,
		Description:   "High-speed AI models gateway (plk_...)",
	},
	{
		ID:            DefaultOllamaName,
		Name:          "Ollama (Local)",
		Protocol:      ProviderProtocolOpenAI,
		BaseURL:       DefaultOllamaEndpoint,
		EndpointHosts: []string{"localhost", "127.0.0.1"},
		RequiresKey:   false,
		Description:   "Local LLM inference, zero cloud cost",
	},
	{
		ID:            DefaultOpenAIName,
		Name:          "OpenAI Official",
		Protocol:      ProviderProtocolOpenAI,
		BaseURL:       DefaultOpenAIEndpoint,
		EndpointHosts: []string{"api.openai.com"},
		RequiresKey:   true,
		Description:   "Direct OpenAI API access (sk-...)",
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
// for a known provider.
func ResolveProviderBaseURL(providerName string, configuredURL string) string {
	if baseURL := strings.TrimSpace(configuredURL); baseURL != "" {
		return strings.TrimRight(baseURL, "/")
	}

	if preset := LookupPreset(providerName); preset != nil {
		return preset.BaseURL
	}
	return DefaultProtonmanEndpoint
}

// RemoteModel describes a model discovered from an OpenAI or protonman endpoint.
type RemoteModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ContextWindow int      `json:"context_window,omitempty"`
	Provider      string   `json:"provider,omitempty"`
	Features      []string `json:"features,omitempty"`
}

// IsFreeModel reports whether a given model ID represents an OpenCode free-tier model.
func IsFreeModel(id string) bool {
	idLower := strings.ToLower(strings.TrimSpace(id))
	return strings.HasSuffix(idLower, "-free") || idLower == "big-pickle"
}

// FetchProviderModels queries a provider's model endpoint to list available models.
func FetchProviderModels(ctx context.Context, baseURL string, apiKey string) ([]RemoteModel, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultProtonmanEndpoint
	}

	client := &http.Client{Timeout: runtimepolicy.ModelDiscoveryTimeout}

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
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &openAIResp); err == nil && len(openAIResp.Data) > 0 {
		results := make([]RemoteModel, 0, len(openAIResp.Data))
		for _, item := range openAIResp.Data {
			name := item.Name
			if name == "" {
				name = item.ID
			}
			results = append(results, RemoteModel{
				ID:   item.ID,
				Name: name,
			})
		}
		return results, nil
	}

	// Attempt parsing protonman format: {"models": [{"slug": "...", "name": "...", "contextWindow": 1000000}]}
	var protonmanResp struct {
		Models []struct {
			ID            string   `json:"id"`
			Slug          string   `json:"slug"`
			Name          string   `json:"name"`
			ContextWindow int      `json:"contextWindow"`
			Features      []string `json:"features"`
			Provider      struct {
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
			results = append(results, RemoteModel{
				ID:            id,
				Name:          item.Name,
				ContextWindow: item.ContextWindow,
				Provider:      item.Provider.Name,
				Features:      item.Features,
			})
		}
		return results, nil
	}

	return nil, errors.New("unrecognized models response format")
}
