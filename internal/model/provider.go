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
)

const (
	// DefaultProtonmanName is the canonical provider label for protonmanAI.
	DefaultProtonmanName = "protonman"
	// DefaultProtonmanEndpoint is the public API Gateway base URL.
	DefaultProtonmanEndpoint = "https://protonman.dev/api/v1"

	// DefaultOpenCodeName is the canonical provider label for OpenCode Zen.
	DefaultOpenCodeName = "opencode"
	// DefaultOpenCodeEndpoint is the base URL for OpenCode Zen API.
	DefaultOpenCodeEndpoint = "https://opencode.ai/zen/v1"
)

// SupportedProviderPreset describes an out-of-the-box model provider preset.
type SupportedProviderPreset struct {
	ID           string
	Name         string
	BaseURL      string
	RequiresKey  bool
	Description  string
	DefaultModel string
}

// SupportedPresets lists available provider presets for discovery and quick setup.
var SupportedPresets = []SupportedProviderPreset{
	{
		ID:           DefaultOpenCodeName,
		Name:         "OpenCode (Free)",
		BaseURL:      DefaultOpenCodeEndpoint,
		RequiresKey:  false,
		Description:  "Free tier models, zero API key required",
		DefaultModel: "nemotron-3.5-lightning-free",
	},
	{
		ID:           DefaultProtonmanName,
		Name:         "Protonman",
		BaseURL:      DefaultProtonmanEndpoint,
		RequiresKey:  true,
		Description:  "High-speed AI models gateway (plk_...)",
		DefaultModel: "deepseek-v4-flash-vision-exp",
	},
	{
		ID:           "ollama",
		Name:         "Ollama (Local)",
		BaseURL:      "http://localhost:11434/v1",
		RequiresKey:  false,
		Description:  "Local LLM inference, zero cloud cost",
		DefaultModel: "llama3.2",
	},
	{
		ID:           "openai",
		Name:         "OpenAI Official",
		BaseURL:      "https://api.openai.com/v1",
		RequiresKey:  true,
		Description:  "Direct OpenAI API access (sk-...)",
		DefaultModel: "gpt-4o",
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

// RemoteModel describes a model discovered from an OpenAI or protonman endpoint.
type RemoteModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ContextWindow int      `json:"context_window,omitempty"`
	Provider      string   `json:"provider,omitempty"`
	Features      []string `json:"features,omitempty"`
}

// DefaultProtonmanModels provides the standard catalog when offline or fallback.
var DefaultProtonmanModels = []RemoteModel{
	{ID: "deepseek-v4-flash-vision-exp", Name: "DeepSeek V4 Flash Vision (exp)", ContextWindow: 1000000, Provider: "DeepSeek", Features: []string{"json", "tools", "vision"}},
	{ID: "glm-5.3-flash", Name: "GLM-5.3 Flash", ContextWindow: 1048576, Provider: "GLM", Features: []string{"tools", "vision"}},
	{ID: "Qwen3.8-Flash", Name: "Qwen 3.8 Flash", ContextWindow: 1000000, Provider: "Qwen", Features: []string{"text", "vision"}},
	{ID: "muse-spark-1.3-contributor", Name: "Muse Spark 1.3 Contributor", ContextWindow: 1048576, Provider: "Meta", Features: []string{"tools", "vision"}},
	{ID: "MiniMax-M3", Name: "MiniMax M3", ContextWindow: 1000000, Provider: "MiniMax", Features: []string{"text"}},
}

// DefaultOpenCodeFreeModels provides the standard free-tier catalog when offline or fallback.
var DefaultOpenCodeFreeModels = []RemoteModel{
	{ID: "nemotron-3.5-lightning-free", Name: "Nemotron 3.5 Lightning (Free)", ContextWindow: 128000, Provider: "NVIDIA", Features: []string{"free", "tools"}},
	{ID: "big-pickle", Name: "Big Pickle (Free)", ContextWindow: 128000, Provider: "OpenCode", Features: []string{"free"}},
	{ID: "mimo-v2.5-free", Name: "MiMo V2.5 (Free)", ContextWindow: 128000, Provider: "MiMo", Features: []string{"free"}},
	{ID: "nemotron-3-ultra-free", Name: "Nemotron 3 Ultra (Free)", ContextWindow: 128000, Provider: "NVIDIA", Features: []string{"free"}},
	{ID: "deepseek-v4-flash-free", Name: "DeepSeek V4 Flash (Free)", ContextWindow: 128000, Provider: "DeepSeek", Features: []string{"free"}},
	{ID: "muse-spark-1.3-contributor-free", Name: "Muse Spark 1.3 Contributor (Free)", ContextWindow: 128000, Provider: "Meta", Features: []string{"free"}},
	{ID: "muse-spark-1.2-contributor-free", Name: "Muse Spark 1.2 Contributor (Free)", ContextWindow: 128000, Provider: "Meta", Features: []string{"free"}},
	{ID: "ling-3.0-flash-fin-free", Name: "Ling 3.0 Flash Fin (Free)", ContextWindow: 128000, Provider: "Ling", Features: []string{"free"}},
	{ID: "laguna-s-2.1-free", Name: "Laguna S 2.1 (Free)", ContextWindow: 128000, Provider: "Laguna", Features: []string{"free"}},
}

// IsFreeModel reports whether a given model ID represents an OpenCode free-tier model.
func IsFreeModel(id string) bool {
	idLower := strings.ToLower(strings.TrimSpace(id))
	return strings.HasSuffix(idLower, "-free") || idLower == "big-pickle"
}

// NormalizeModelID cleans and harmonizes known model ID typos and provider-specific suffixes.
func NormalizeModelID(endpointOrProvider string, modelID string) string {
	raw := strings.TrimSpace(modelID)
	if raw == "" {
		return raw
	}

	lower := strings.ToLower(raw)
	// Fix common typo: contributer -> contributor
	if strings.Contains(lower, "contributer") {
		raw = strings.ReplaceAll(raw, "contributer", "contributor")
		raw = strings.ReplaceAll(raw, "Contributer", "Contributor")
		lower = strings.ToLower(raw)
	}

	// For OpenCode: ensure free-tier models have the -free suffix
	isOpencode := strings.Contains(strings.ToLower(endpointOrProvider), "opencode")
	if isOpencode && !strings.HasSuffix(lower, "-free") && lower != "big-pickle" {
		knownFreeBases := []string{
			"nemotron-3.5-lightning",
			"nemotron-3-ultra",
			"mimo-v2.5",
			"deepseek-v4-flash",
			"muse-spark-1.3-contributor",
			"muse-spark-1.2-contributor",
			"ling-3.0-flash-fin",
			"laguna-s-2.1",
		}
		for _, base := range knownFreeBases {
			if strings.EqualFold(raw, base) {
				return base + "-free"
			}
		}
	}

	return raw
}

// FetchProviderModels queries a provider's model endpoint to list available models.
func FetchProviderModels(ctx context.Context, baseURL string, apiKey string) ([]RemoteModel, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultProtonmanEndpoint
	}

	client := &http.Client{Timeout: 10 * time.Second}

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

	// 3. Check domain fallbacks if remote request failed
	if strings.Contains(baseURL, "protonman.dev") {
		return append([]RemoteModel{}, DefaultProtonmanModels...), nil
	}
	if strings.Contains(baseURL, "opencode.ai") {
		return append([]RemoteModel{}, DefaultOpenCodeFreeModels...), nil
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
